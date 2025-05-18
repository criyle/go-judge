package restexecutor

import "github.com/gin-gonic/gin"

// Register registers executor the handler
type Register interface {
	Register(*gin.Engine)
}
