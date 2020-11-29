package main

import (
	"net/http"
	"runtime"

	"github.com/criyle/go-judge/cmd/executorserver/model"
	"github.com/criyle/go-judge/worker"
	"github.com/gin-gonic/gin"
)

type cmdHandle struct {
	worker    worker.Worker
	srcPrefix string
}

func (h *cmdHandle) handleRun(c *gin.Context) {
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
	logger.Sugar().Debugf("request: %+v", r)
	rt := <-h.worker.Submit(c.Request.Context(), r)
	logger.Sugar().Debugf("response: %+v", rt)
	execObserve(rt)
	if rt.Error != nil {
		c.Error(rt.Error)
		c.AbortWithStatusJSON(http.StatusInternalServerError, rt.Error.Error())
		return
	}
	c.JSON(http.StatusOK, model.ConvertResponse(rt).Results)
}

func handleVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"buildVersion": Version,
		"goVersion":    runtime.Version(),
		"platform":     runtime.GOARCH,
		"os":           runtime.GOOS,
	})
}
