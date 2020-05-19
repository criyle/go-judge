// Command executorserver will starts a http server that receives command to run
// programs inside a sandbox.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/criyle/go-judge/env"
	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/pkg/pool"
	"github.com/criyle/go-judge/worker"
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

	printLog = func(v ...interface{}) {}

	work *worker.Worker
)

func newFilsStore(dir string) filestore.FileStore {
	var fs filestore.FileStore
	if dir == "" {
		fs = filestore.NewFileMemoryStore()
	} else {
		os.MkdirAll(dir, 0755)
		fs = filestore.NewFileLocalStore(dir)
	}
	return fs
}

func main() {
	flag.Parse()

	if !*silent {
		printLog = log.Println
	}

	fs := newFilsStore(*dir)
	b, err := env.NewBuilder(*cinitPath, *mountConf, *tmpFsParam, *netShare, printLog)
	if err != nil {
		log.Fatalln("create environment builder failed", err)
	}
	envPool := pool.NewPool(b)
	work = worker.New(fs, envPool, *parallism, *dir)
	work.Start()
	printLog("Starting worker with parallism", *parallism)

	var r *gin.Engine
	if *silent {
		gin.SetMode(gin.ReleaseMode)
		r = gin.New()
		r.Use(gin.Recovery())
	} else {
		r = gin.Default()
	}

	// File Handles
	fh := &fileHandle{fs: fs}
	r.GET("/file", fh.fileGet)
	r.POST("/file", fh.filePost)
	r.GET("/file/:fid", fh.fileIDGet)
	r.DELETE("/file/:fid", fh.fileIDDelete)

	// Run Handle
	r.POST("/run", handleRun)

	// WebSocket Handle
	r.GET("/ws", handleWS)

	printLog("Starting http server at", *addr)
	r.Run(*addr)
}
