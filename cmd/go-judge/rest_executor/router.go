package restexecutor

import (
	"github.com/gin-gonic/gin"
)

func RegisterRouter(r *gin.Engine, fileHandler *FileHandler, cmdHandler *CmdHandler) {
	// Run CmdHandler
	r.POST("/run", cmdHandler.handleRun)

	// File CmdHandler
	r.GET("/file", fileHandler.fileGet)
	r.POST("/file", fileHandler.filePost)
	r.GET("/file/:fid", fileHandler.fileIDGet)
	r.DELETE("/file/:fid", fileHandler.fileIDDelete)
}
