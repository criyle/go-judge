package wsexecutor

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/criyle/go-judge/cmd/go-judge/model"
	"github.com/criyle/go-judge/cmd/go-judge/stream"
	"github.com/criyle/go-judge/worker"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var _ Register = &wsHandle{}

// Register registers web socket handle /ws
type Register interface {
	Register(*gin.Engine)
}

// New creates new websocket handle
func New(worker worker.Worker, srcPrefix []string, logger *zap.Logger) Register {
	return &wsHandle{
		worker:    worker,
		srcPrefix: srcPrefix,
		logger:    logger,
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 50 * time.Second
)

type wsHandle struct {
	worker    worker.Worker
	srcPrefix []string
	logger    *zap.Logger
}

type wsRequest struct {
	model.Request
	CancelRequestID string `json:"cancelRequestId"`
}

func (h *wsHandle) Register(r *gin.Engine) {
	r.GET("/ws", h.handleWS)
	r.GET("/stream", h.handleStream)
}

func (h *wsHandle) handleWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.Error(err)
		c.AbortWithStatusJSON(http.StatusBadRequest, err.Error())
		return
	}
	resultCh := make(chan model.Response, 128)
	cm := newContextMap()

	handleRequest := func(baseCtx context.Context, req *wsRequest) error {
		if req.CancelRequestID != "" {
			h.logger.Debug("ws cancel", zap.String("requestId", req.CancelRequestID))
			cm.Remove(req.CancelRequestID)
			return nil
		}
		r, err := model.ConvertRequest(&req.Request, h.srcPrefix)
		if err != nil {
			return fmt.Errorf("ws convert error: %w", err)
		}

		ctx, cancel := context.WithCancel(baseCtx)
		if err := cm.Add(r.RequestID, cancel); err != nil {
			select {
			case <-baseCtx.Done():
			case resultCh <- model.Response{
				RequestID: req.RequestID,
				ErrorMsg:  err.Error(),
			}:
			}
			cancel()
			h.logger.Debug("ws request error", zap.Error(err))
			return nil
		}

		go func() {
			defer cm.Remove(r.RequestID)

			if ce := h.logger.Check(zap.DebugLevel, "ws request"); ce != nil {
				ce.Write(zap.String("body", fmt.Sprintf("%+v", r)))
			}
			retCh, started := h.worker.Submit(ctx, r)
			var ret worker.Response
			select {
			case <-baseCtx.Done(): // if connection lost
				return
			case <-ctx.Done(): // if context cancelled by cancelling request
				select {
				case <-started: // if started, wait for result
					ret = <-retCh
				default: // not started
					ret = worker.Response{
						RequestID: r.RequestID,
						Error:     fmt.Errorf("request cancelled before execute"),
					}
				}
			case ret = <-retCh:
			}
			if ce := h.logger.Check(zap.DebugLevel, "response"); ce != nil {
				ce.Write(zap.String("body", fmt.Sprintf("%+v", ret)))
			}

			resp, err := model.ConvertResponse(ret, false)
			if err != nil {
				resp = model.Response{
					RequestID: r.RequestID,
					ErrorMsg:  resp.ErrorMsg,
				}
			}
			select {
			case <-baseCtx.Done():
			case resultCh <- resp:
			}
		}()
		return nil
	}

	// read request
	go func() {
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})

		baseCtx, baseCancel := context.WithCancel(context.TODO())
		defer baseCancel()

		for {
			req := new(wsRequest)
			if err := conn.ReadJSON(req); err != nil {
				h.logger.Info("ws read error", zap.Error(err))
				return
			}
			if err := handleRequest(baseCtx, req); err != nil {
				h.logger.Info("ws handle error", zap.Error(err))
				return
			}
		}
	}()

	// write result
	go func() {
		defer conn.Close()
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case r := <-resultCh:
				conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteJSON(r); err != nil {
					h.logger.Info("ws write error", zap.Error(err))
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()
}

func (h *wsHandle) handleStream(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.Error(err)
		c.AbortWithStatusJSON(http.StatusBadRequest, err.Error())
		return
	}
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	w := &streamWrapper{ctx: ctx, conn: conn, sendCh: make(chan stream.Response)}
	go w.sendLoop()
	if err := stream.Start(ctx, w, h.worker, h.srcPrefix, h.logger); err != nil {
		h.logger.Debug("stream start", zap.Error(err))
		c.Error(err)
	}
}

type contextMap struct {
	m  map[string]context.CancelFunc
	mu sync.Mutex
}

func newContextMap() *contextMap {
	return &contextMap{m: make(map[string]context.CancelFunc)}
}

func (c *contextMap) Add(reqID string, cancel context.CancelFunc) error {
	if reqID == "" {
		return fmt.Errorf("empty request id")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exist := c.m[reqID]; exist {
		return fmt.Errorf("duplicated request id: %q", reqID)
	}
	c.m[reqID] = cancel
	return nil
}

func (c *contextMap) Remove(reqID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cancel, exist := c.m[reqID]; exist {
		delete(c.m, reqID)
		cancel()
	}
}
