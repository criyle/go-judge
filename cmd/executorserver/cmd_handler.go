package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func handleRun(c *gin.Context) {
	var req request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(err)
		c.AbortWithStatusJSON(http.StatusBadRequest, err.Error())
		return
	}

	if len(req.Cmd) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, "no cmd provided")
		return
	}
	ret := submitRequest(&req)
	rt := <-ret
	if rt.Error != nil {
		c.Error(rt.Error)
		c.AbortWithStatusJSON(http.StatusInternalServerError, rt.Error.Error())
		return
	}
	c.JSON(http.StatusOK, rt.Response)
}
