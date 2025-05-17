package restexecutor

import (
	"encoding/json"
	"net/http"

	"github.com/criyle/go-judge/cmd/go-judge/model"
	"github.com/criyle/go-judge/worker"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type CmdHandler struct {
	worker    worker.Worker
	srcPrefix []string
	logger    *zap.Logger
}

func NewCmdHandler(worker worker.Worker, srcPrefix []string, logger *zap.Logger) *CmdHandler {
	return &CmdHandler{
		worker:    worker,
		srcPrefix: srcPrefix,
		logger:    logger,
	}
}

func (h *CmdHandler) handleRun(c *gin.Context) {
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
