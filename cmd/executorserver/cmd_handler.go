package main

import (
	"net/http"

	"github.com/criyle/go-judge/worker"
	"github.com/gin-gonic/gin"
)

func handleRun(c *gin.Context) {
	var req worker.Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(err)
		c.AbortWithStatusJSON(http.StatusBadRequest, err.Error())
		return
	}

	if len(req.Cmd) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, "no cmd provided")
		return
	}
	rt := <-work.Submit(&req)
	if rt.Error != nil {
		c.Error(rt.Error)
		c.AbortWithStatusJSON(http.StatusInternalServerError, rt.Error.Error())
		return
	}
	c.JSON(http.StatusOK, rt.Response)
}
