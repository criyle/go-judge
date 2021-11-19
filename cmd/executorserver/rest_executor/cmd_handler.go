package restexecutor

import (
	"encoding/json"
	"net/http"

	"github.com/criyle/go-judge/cmd/executorserver/model"
	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/worker"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Register registers executor the handler
//
// POST /run, GET /file, POST /file, GET /file/:fid, DELETE /file/:fid
type Register interface {
	Register(*gin.Engine)
}

// New creates new REST API handler
func New(worker worker.Worker, fs filestore.FileStore, srcPrefix string, logger *zap.Logger) Register {
	return &handle{
		worker:     worker,
		fileHandle: fileHandle{fs: fs},
		srcPrefix:  srcPrefix,
		logger:     logger,
	}
}

type handle struct {
	worker worker.Worker
	fileHandle
	srcPrefix string
	logger    *zap.Logger
}

func (h *handle) Register(r *gin.Engine) {
	// Run handle
	r.POST("/run", h.handleRun)

	// File handle
	r.GET("/file", h.fileGet)
	r.POST("/file", h.filePost)
	r.GET("/file/:fid", h.fileIDGet)
	r.DELETE("/file/:fid", h.fileIDDelete)
}

func (h *handle) handleRun(c *gin.Context) {
	var req model.Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(err)
		c.AbortWithStatusJSON(http.StatusBadRequest, err.Error())
		return
	}

	if len(req.Cmd) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, "no cmd provided")
		return
	}
	r, err := model.ConvertRequest(&req, h.srcPrefix)
	if err != nil {
		c.Error(err)
		c.AbortWithStatusJSON(http.StatusBadRequest, err.Error())
		return
	}
	h.logger.Sugar().Debugf("request: %+v", r)
	rtCh, _ := h.worker.Submit(c.Request.Context(), r)
	rt := <-rtCh
	h.logger.Sugar().Debugf("response: %+v", rt)
	if rt.Error != nil {
		c.Error(rt.Error)
		c.AbortWithStatusJSON(http.StatusInternalServerError, rt.Error.Error())
		return
	}

	// encode json directly to avoid allocation
	c.Status(http.StatusOK)
	c.Header("Content-Type", "application/json; charset=utf-8")

	res, err := model.ConvertResponse(rt, true)
	if err != nil {
		c.Error(err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
		return
	}
	defer res.Close()

	if err := json.NewEncoder(c.Writer).Encode(res.Results); err != nil {
		c.Error(err)
	}
}
