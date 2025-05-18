package restexecutor

import (
	"encoding/json"
	"net/http"

	"github.com/criyle/go-judge/cmd/go-judge/model"
	"github.com/criyle/go-judge/worker"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type cmdHandle struct {
	worker    worker.Worker
	srcPrefix []string
	logger    *zap.Logger
}

// NewCmdHandle creates a new command handle
func NewCmdHandle(worker worker.Worker, srcPrefix []string, logger *zap.Logger) Register {
	return &cmdHandle{
		worker:    worker,
		srcPrefix: srcPrefix,
		logger:    logger,
	}
}

func (c *cmdHandle) Register(r *gin.Engine) {
	// Run handle
	r.POST("/run", c.handleRun)
}

func (c *cmdHandle) handleRun(ctx *gin.Context) {
	var req model.Request
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.Error(err)
		ctx.AbortWithStatusJSON(http.StatusBadRequest, err.Error())
		return
	}

	if len(req.Cmd) == 0 {
		ctx.AbortWithStatusJSON(http.StatusBadRequest, "no cmd provided")
		return
	}
	r, err := model.ConvertRequest(&req, c.srcPrefix)
	if err != nil {
		ctx.Error(err)
		ctx.AbortWithStatusJSON(http.StatusBadRequest, err.Error())
		return
	}
	c.logger.Sugar().Debugf("request: %+v", r)
	rtCh, _ := c.worker.Submit(ctx.Request.Context(), r)
	rt := <-rtCh
	c.logger.Sugar().Debugf("response: %+v", rt)
	if rt.Error != nil {
		ctx.Error(rt.Error)
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, rt.Error.Error())
		return
	}

	// encode json directly to avoid allocation
	ctx.Status(http.StatusOK)
	ctx.Header("Content-Type", "application/json; charset=utf-8")

	res, err := model.ConvertResponse(rt, true)
	if err != nil {
		ctx.Error(err)
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
		return
	}
	defer res.Close()

	if err := json.NewEncoder(ctx.Writer).Encode(res.Results); err != nil {
		ctx.Error(err)
	}
}
