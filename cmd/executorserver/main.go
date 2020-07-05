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
	"strconv"
	"strings"
	"time"

	"github.com/criyle/go-judge/env"
	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/pb"
	"github.com/criyle/go-judge/pkg/pool"
	"github.com/criyle/go-judge/worker"
	ginpprof "github.com/gin-contrib/pprof"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	ginprometheus "github.com/zsais/go-gin-prometheus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	envDebug   = "DEBUG"
	envMetrics = "METRICS"

	envAddr      = "HTTP_ADDR"
	envGRPC      = "GRPC"
	envGRPCAddr  = "GRPC_ADDR"
	envParallism = "PARALLISM"
	envToken     = "TOKEN"
	envRelease   = "RELEASE"
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
	token      = flag.String("token", "", "bearer token auth for REST / gRPC")
	release    = flag.Bool("release", false, "use release mode for log")
	srcPrefix  = flag.String("srcprefix", "", "specifies directory prefix for source type copyin")

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

func initEnv() (bool, error) {
	eneableGRPC := false
	if s := os.Getenv(envAddr); s != "" {
		addr = &s
	}
	if os.Getenv(envGRPC) == "1" {
		eneableGRPC = true
	}
	if s := os.Getenv(envGRPCAddr); s != "" {
		eneableGRPC = true
		grpcAddr = &s
	}
	if s := os.Getenv(envParallism); s != "" {
		p, err := strconv.Atoi(s)
		if err != nil {
			return false, err
		}
		parallism = &p
	}
	if s := os.Getenv(envToken); s != "" {
		token = &s
	}
	if os.Getpid() == 1 || os.Getenv(envRelease) == "1" {
		*release = true
	}
	return eneableGRPC, nil
}

func main() {
	flag.Parse()

	enableGRPC, err := initEnv()
	if err != nil {
		log.Fatalln("init environment variable failed", err)
	}

	var logger *zap.Logger
	if !*silent {
		if *release {
			logger, err = zap.NewProduction()
		} else {
			config := zap.NewDevelopmentConfig()
			config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
			logger, err = config.Build()
		}
		if err != nil {
			log.Fatalln("init logger failed", err)
		}
		defer logger.Sync()

		printLog = logger.Sugar().Info
	} else {
		logger = zap.NewNop()
	}

	// Init environment pool
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
	if *release {
		gin.SetMode(gin.ReleaseMode)
	}
	r = gin.New()
	if *silent {
		r.Use(gin.Recovery())
	} else {
		r.Use(ginzap.Ginzap(logger, time.RFC3339, true))
		r.Use(ginzap.RecoveryWithZap(logger, true))
	}

	// Metrics Handle
	if os.Getenv(envMetrics) == "1" {
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
	}

	// Add auth token
	if *token != "" {
		r.Use(tokenAuth(*token))
		printLog("Attach token auth with token:", *token)
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

	// pprof
	if os.Getenv(envDebug) != "" {
		ginpprof.Register(r)
	}

	// gRPC server
	var grpcServer *grpc.Server
	if enableGRPC {
		grpc_zap.ReplaceGrpcLoggerV2(logger)
		streamMiddleware := []grpc.StreamServerInterceptor{
			grpc_prometheus.StreamServerInterceptor,
			grpc_zap.StreamServerInterceptor(logger),
			grpc_recovery.StreamServerInterceptor(),
		}
		unaryMiddleware := []grpc.UnaryServerInterceptor{
			grpc_prometheus.UnaryServerInterceptor,
			grpc_zap.UnaryServerInterceptor(logger),
			grpc_recovery.UnaryServerInterceptor(),
		}
		if *token != "" {
			authFunc := grpcTokenAuth(*token)
			streamMiddleware = append(streamMiddleware, grpc_auth.StreamServerInterceptor(authFunc))
			unaryMiddleware = append(unaryMiddleware, grpc_auth.UnaryServerInterceptor(authFunc))
		}
		grpcServer = grpc.NewServer(
			grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(streamMiddleware...)),
			grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryMiddleware...)),
		)
		pb.RegisterExecutorServer(grpcServer, &execServer{fs: fs, srcPrefix: *srcPrefix})
		grpc_prometheus.Register(grpcServer)
		grpc_prometheus.EnableHandlingTimeHistogram()

		lis, err := net.Listen("tcp", *grpcAddr)
		if err != nil {
			log.Fatalln(err)
		}
		go func() {
			printLog("Starting grpc server at", *grpcAddr)
			printLog("GRPC serve", grpcServer.Serve(lis))
		}()
	}

	srv := http.Server{
		Addr:    *addr,
		Handler: r,
	}

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

	if grpcServer != nil {
		eg.Go(func() error {
			grpcServer.GracefulStop()
			printLog("GRPC server shutdown")
			return nil
		})
	}

	go func() {
		printLog("Shutdown Finished", eg.Wait())
		cancel()
	}()
	<-ctx.Done()
}

func tokenAuth(token string) gin.HandlerFunc {
	const bearer = "Bearer "
	return func(c *gin.Context) {
		reqToken := c.GetHeader("Authorization")
		if strings.HasPrefix(reqToken, bearer) && reqToken[len(bearer):] == token {
			c.Next()
			return
		}
		c.AbortWithStatus(http.StatusUnauthorized)
	}
}

func grpcTokenAuth(token string) func(context.Context) (context.Context, error) {
	return func(ctx context.Context) (context.Context, error) {
		reqToken, err := grpc_auth.AuthFromMD(ctx, "bearer")
		if err != nil {
			return nil, err
		}
		if reqToken != token {
			return nil, status.Errorf(codes.Unauthenticated, "invalid auth token: %v", err)
		}
		return ctx, nil
	}
}
