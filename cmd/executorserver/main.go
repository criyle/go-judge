// Command executorserver will starts a http server that receives command to run
// programs inside a sandbox.
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/criyle/go-judge/env"
	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/pb"
	"github.com/criyle/go-judge/pkg/pool"
	"github.com/criyle/go-judge/worker"
	ginpprof "github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	ginprometheus "github.com/zsais/go-gin-prometheus"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

var (
	addr       = flag.String("http", ":5050", "specifies the http binding address")
	grpcAddr   = flag.String("grpc", ":5051", "specifies the grpc binding address")
	parallism  = flag.Int("parallism", 4, "control the # of concurrency execution")
	tmpFsParam = flag.String("tmpfs", "size=16m,nr_inodes=4k", "tmpfs mount data (only for default mount with no mount.yaml)")
	dir        = flag.String("dir", "", "specifies directory to store file upload / download (in memory by default)")
	silent     = flag.Bool("silent", false, "do not print logs")
	netShare   = flag.Bool("net", false, "do not unshare net namespace with host")
	mountConf  = flag.String("mount", "mount.yaml", "specifics mount configuration file")
	cinitPath  = flag.String("cinit", "", "container init absolute path")

	cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")

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

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer f.Close() // error handling omitted for example
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

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

	// Metrics Handle
	p := ginprometheus.NewPrometheus("gin")
	p.ReqCntURLLabelMappingFn = func(c *gin.Context) string {
		url := c.Request.URL.Path
		for _, p := range c.Params {
			if p.Key == "fid" {
				url = strings.Replace(url, p.Value, ":fid", 1)
			}
		}
		return url
	}
	p.Use(r)

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

	// pprof
	ginpprof.Register(r)

	// gRPC server
	grpcServer := grpc.NewServer(
		grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
		grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor),
	)
	pb.RegisterExecutorServer(grpcServer, &execServer{fs: fs})
	grpc_prometheus.Register(grpcServer)
	grpc_prometheus.EnableHandlingTimeHistogram()

	lis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		log.Fatalln(err)
	}

	srv := http.Server{
		Addr:    *addr,
		Handler: r,
	}

	go func() {
		printLog("Starting grpc server at", *grpcAddr)
		printLog("GRPC serve", grpcServer.Serve(lis))
	}()

	go func() {
		printLog("Starting http server at", *addr)
		printLog("Http serve", srv.ListenAndServe())
	}()

	// Graceful shutdown...
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig

	printLog("Shutting Down...")

	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*3)
	defer cancel()

	var eg errgroup.Group
	eg.Go(func() error {
		printLog("Http server shutdown")
		return srv.Shutdown(ctx)
	})

	eg.Go(func() error {
		work.Shutdown()
		printLog("Worker shutdown")
		return nil
	})

	eg.Go(func() error {
		grpcServer.GracefulStop()
		printLog("GRPC server shutdown")
		return nil
	})

	go func() {
		printLog("Shutdown Finished", eg.Wait())
		cancel()
	}()
	<-ctx.Done()
}
