// Command executorserver will starts a http server that receives command to run
// programs inside a sandbox.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/criyle/go-judge/pkg/envexec"
	"github.com/gin-gonic/gin"
)

var (
	addr       = flag.String("http", ":5050", "specifies the http binding address")
	parallism  = flag.Int("parallism", 4, "control the # of concurrency execution")
	tmpFsParam = flag.String("tmpfs", "size=16m,nr_inodes=4k", "tmpfs mount data (only for default mount with no mount.yaml)")
	dir        = flag.String("dir", "", "specifies directory to store file upload / download (in memory by default)")
	silent     = flag.Bool("silent", false, "do not print logs")
	netShare   = flag.Bool("net", false, "do not unshare net namespace with host")
	mountConf  = flag.String("mount", "mount.yaml", "specifics mount configuration file")
	cinitPath  = flag.String("cinit", "", "container init absolute path")

	envPool envexec.EnvironmentPool

	fs fileStore

	printLog = log.Println
)

func main() {
	flag.Parse()

	if *dir == "" {
		fs = newFileMemoryStore()
	} else {
		os.MkdirAll(*dir, 0755)
		fs = newFileLocalStore(*dir)
	}
	if *silent {
		printLog = func(v ...interface{}) {}
	}

	initEnvPool()

	printLog("Starting worker with parallism", *parallism)
	startWorkers()

	var r *gin.Engine
	if *silent {
		gin.SetMode(gin.ReleaseMode)
		r = gin.New()
		r.Use(gin.Recovery())
	} else {
		r = gin.Default()
	}
	r.GET("/file", fileGet)
	r.POST("/file", filePost)
	r.GET("/file/:fid", fileIDGet)
	r.DELETE("/file/:fid", fileIDDelete)
	r.POST("/run", handleRun)

	r.GET("/ws", handleWS)

	printLog("Starting http server at", *addr)
	r.Run(*addr)
}
