package wsexecutor

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/criyle/go-judge/cmd/executorserver/model"
	"github.com/criyle/go-judge/worker"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// Register registers web socket handle /ws
type Register interface {
	Register(*gin.Engine)
}

// New creates new websocket handle
func New(worker worker.Worker, srcPrefix string, logger *zap.Logger) Register {
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
	srcPrefix string
	logger    *zap.Logger
}

type wsRequest struct {
	model.Request
	CancelRequestId string `json:"cancelRequestId"`
}

func (h *wsHandle) Register(r *gin.Engine) {
	r.GET("/ws", h.handleWS)
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
		if req.CancelRequestId != "" {
			h.logger.Sugar().Debugf("ws cancel: %s", req.CancelRequestId)
			cm.Remove(req.CancelRequestId)
			return nil
		}
		r, err := model.ConvertRequest(&req.Request, h.srcPrefix)
		if err != nil {
			return fmt.Errorf("ws convert error: %v", err)
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
			h.logger.Sugar().Debugf("ws request error: %v", err)
			return nil
		}

		go func() {
			defer cm.Remove(r.RequestID)

			h.logger.Sugar().Debugf("ws request: %+v", r)
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
			h.logger.Sugar().Debugf("ws response: %+v", ret)

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
				h.logger.Sugar().Info("ws read error:", err)
				return
			}
			if err := handleRequest(baseCtx, req); err != nil {
				h.logger.Sugar().Info("ws handle error:", err)
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
					h.logger.Sugar().Info("ws write error:", err)
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

type contextMap struct {
	m  map[string]context.CancelFunc
	mu sync.Mutex
}

func newContextMap() *contextMap {
	return &contextMap{m: make(map[string]context.CancelFunc)}
}

func (c *contextMap) Add(reqId string, cancel context.CancelFunc) error {
	if reqId == "" {
		return fmt.Errorf("empty request id")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exist := c.m[reqId]; exist {
		return fmt.Errorf("duplicated request id: %v", reqId)
	}
	c.m[reqId] = cancel
	return nil
}

func (c *contextMap) Remove(reqId string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cancel, exist := c.m[reqId]; exist {
		delete(c.m, reqId)
		cancel()
	}
}
