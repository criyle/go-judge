package main

import (
	"net/http"

	"github.com/criyle/go-judge/cmd/executorserver/model"
	"github.com/gin-gonic/gin"
)

func handleRun(c *gin.Context) {
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
	r, err := model.ConvertRequest(&req)
	if err != nil {
		c.Error(err)
		c.AbortWithStatusJSON(http.StatusBadRequest, err.Error)
		return
	}
	rt := <-work.Submit(c.Request.Context(), r)
	execObserve(rt)
	if rt.Error != nil {
		c.Error(rt.Error)
		c.AbortWithStatusJSON(http.StatusInternalServerError, rt.Error.Error())
		return
	}
	c.JSON(http.StatusOK, model.ConvertResponse(rt).Results)
}
