// Command executorserver will starts a http server that receives command to run
// programs inside a sandbox.
package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
	"sync/atomic"
	"syscall"

	"github.com/criyle/go-judge/pkg/envexec"
	"github.com/criyle/go-judge/pkg/pool"
	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/pkg/cgroup"
	"github.com/criyle/go-sandbox/pkg/forkexec"
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

	envPool    envexec.EnvironmentPool
	cgroupPool envexec.CgroupPool

	fs fileStore

	printLog = log.Println
)

func init() {
	container.Init()
}

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

	root, err := ioutil.TempDir("", "dm")
	if err != nil {
		log.Fatalln(err)
	}
	printLog("Created tmp dir for container root at:", root)

	mb, err := parseMountConfig(*mountConf)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Fatalln(err)
		}
		printLog("Use the default container mount")
		mb = getDefaultMount()
	}
	m, err := mb.Build(true)
	if err != nil {
		log.Fatalln(err)
	}
	printLog("Created container mount at:", mb)

	unshareFlags := uintptr(forkexec.UnshareFlags)
	if *netShare {
		unshareFlags ^= syscall.CLONE_NEWNET
	}

	b := &container.Builder{
		Root:          root,
		Mounts:        m,
		CredGenerator: newCredGen(),
		Stderr:        true,
		CloneFlags:    unshareFlags,
	}
	cgb, err := cgroup.NewBuilder("executorserver").WithCPUAcct().WithMemory().WithPids().FilterByEnv()
	if err != nil {
		log.Fatalln(err)
	}
	printLog("Created cgroup builder with:", cgb)

	envPool = pool.NewEnvPool(b)
	cgroupPool = pool.NewFakeCgroupPool(cgb)

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

type credGen struct {
	cur uint32
}

func newCredGen() *credGen {
	return &credGen{cur: 10000}
}

func (c *credGen) Get() syscall.Credential {
	n := atomic.AddUint32(&c.cur, 1)
	return syscall.Credential{
		Uid: n,
		Gid: n,
	}
}
