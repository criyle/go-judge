package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func handleRun(c *gin.Context) {
	var req request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	if len(req.Cmd) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, "no cmd provided")
		return
	}
	ret := submitRequest(&req)
	c.JSON(http.StatusOK, <-ret)
}
