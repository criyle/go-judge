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
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/criyle/go-judge/cmd/executorserver/config"
	grpcexecutor "github.com/criyle/go-judge/cmd/executorserver/grpc_executor"
	restexecutor "github.com/criyle/go-judge/cmd/executorserver/rest_executor"
	"github.com/criyle/go-judge/cmd/executorserver/version"
	wsexecutor "github.com/criyle/go-judge/cmd/executorserver/ws_executor"
	"github.com/criyle/go-judge/env"
	"github.com/criyle/go-judge/env/pool"
	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/pb"
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

var logger *zap.Logger

func main() {
	conf := loadConf()
	initLogger(conf)
	logger.Sugar().Infof("config loaded: %+v", conf)

	// Init environment pool
	fs, fsDir, fsCleanUp, err := newFilsStore(conf.Dir, conf.FileTimeout, conf.EnableMetrics)
	if err != nil {
		logger.Sugar().Fatalf("Create temp dir failed %v", err)
	}
	conf.Dir = fsDir
	b := newEnvBuilder(conf)
	envPool := newEnvPool(b, conf.EnableMetrics)
	prefork(envPool, conf.PreFork)
	work := newWorker(conf, envPool, fs)
	work.Start()
	logger.Sugar().Infof("Starting worker with parallelism=%d, workdir=%s, timeLimitCheckInterval=%v",
		conf.Parallelism, conf.Dir, conf.TimeLimitCheckerInterval)

	// Init http handle
	r := initHTTPMux(conf, work, fs)
	srv := http.Server{
		Addr:    conf.HTTPAddr,
		Handler: r,
	}

	// Gracefully shutdown, with signal / HTTP server / gRPC server
	sig := make(chan os.Signal, 3)

	go func() {
		logger.Sugar().Info("Starting http server at ", conf.HTTPAddr)
		logger.Sugar().Info("Http server stopped: ", srv.ListenAndServe())
		sig <- os.Interrupt
	}()

	// Init gRPC server
	var grpcServer *grpc.Server
	if conf.EnableGRPC {
		esServer := grpcexecutor.New(work, fs, conf.SrcPrefix, logger)
		grpcServer = newGRPCServer(conf, esServer)

		lis, err := net.Listen("tcp", conf.GRPCAddr)
		if err != nil {
			log.Fatalln(err)
		}
		go func() {
			logger.Sugar().Info("Starting gRPC server at ", conf.GRPCAddr)
			logger.Sugar().Info("gRPC server stopped: ", grpcServer.Serve(lis))
			sig <- os.Interrupt
		}()
	}

	// background force GC worker
	newForceGCWorker(conf)

	// Graceful shutdown...
	signal.Notify(sig, os.Interrupt)
	<-sig
	signal.Reset(os.Interrupt)

	logger.Sugar().Info("Shutting Down...")

	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*3)
	defer cancel()

	var eg errgroup.Group
	eg.Go(func() error {
		work.Shutdown()
		logger.Sugar().Info("Worker shutdown")
		return nil
	})

	eg.Go(func() error {
		logger.Sugar().Info("Http server shutdown")
		return srv.Shutdown(ctx)
	})

	if grpcServer != nil {
		eg.Go(func() error {
			grpcServer.GracefulStop()
			logger.Sugar().Info("GRPC server shutdown")
			return nil
		})
	}

	if fsCleanUp != nil {
		eg.Go(func() error {
			err := fsCleanUp()
			logger.Sugar().Info("FileStore clean up")
			return err
		})
	}

	go func() {
		logger.Sugar().Info("Shutdown Finished: ", eg.Wait())
		cancel()
	}()
	<-ctx.Done()
}

func loadConf() *config.Config {
	var conf config.Config
	if err := conf.Load(); err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		log.Fatalln("load config failed", err)
	}
	return &conf
}

func initLogger(conf *config.Config) {
	if !conf.Silent {
		var err error
		if conf.Release {
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
	} else {
		logger = zap.NewNop()
	}
}

func prefork(envPool worker.EnvironmentPool, prefork int) {
	if prefork <= 0 {
		return
	}
	logger.Sugar().Info("create ", prefork, " prefork containers")
	m := make([]envexec.Environment, 0, prefork)
	for i := 0; i < prefork; i++ {
		e, err := envPool.Get()
		if err != nil {
			log.Fatalln("prefork environment failed", err)
		}
		m = append(m, e)
	}
	for _, e := range m {
		envPool.Put(e)
	}
}

func initHTTPMux(conf *config.Config, work worker.Worker, fs filestore.FileStore) http.Handler {
	var r *gin.Engine
	if conf.Release {
		gin.SetMode(gin.ReleaseMode)
	}
	r = gin.New()
	if conf.Silent {
		r.Use(gin.Recovery())
	} else {
		r.Use(ginzap.Ginzap(logger, time.RFC3339, true))
		r.Use(ginzap.RecoveryWithZap(logger, true))
	}

	// Metrics Handle
	if conf.EnableMetrics {
		initGinMetrics(r)
	}

	// Version handle
	r.GET("/version", handleVersion)

	// Config handle
	r.GET("/config", generateHandleConfig(conf))

	// Add auth token
	if conf.AuthToken != "" {
		r.Use(tokenAuth(conf.AuthToken))
		logger.Sugar().Info("Attach token auth with token:", conf.AuthToken)
	}

	// Rest Handle
	restHandle := restexecutor.New(work, fs, conf.SrcPrefix, logger)
	restHandle.Register(r)

	// WebSocket Handle
	wsHandle := wsexecutor.New(work, conf.SrcPrefix, logger)
	wsHandle.Register(r)

	// pprof
	if conf.EnableDebug {
		ginpprof.Register(r)
	}
	return r
}

func newGRPCServer(conf *config.Config, esServer pb.ExecutorServer) *grpc.Server {
	var grpcServer *grpc.Server
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
	if conf.AuthToken != "" {
		authFunc := grpcTokenAuth(conf.AuthToken)
		streamMiddleware = append(streamMiddleware, grpc_auth.StreamServerInterceptor(authFunc))
		unaryMiddleware = append(unaryMiddleware, grpc_auth.UnaryServerInterceptor(authFunc))
	}
	grpcServer = grpc.NewServer(
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(streamMiddleware...)),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryMiddleware...)),
	)
	pb.RegisterExecutorServer(grpcServer, esServer)
	grpc_prometheus.Register(grpcServer)
	grpc_prometheus.EnableHandlingTimeHistogram()
	return grpcServer
}

func initGinMetrics(r *gin.Engine) {
	p := ginprometheus.NewPrometheus("gin")
	p.ReqCntURLLabelMappingFn = func(c *gin.Context) string {
		return c.FullPath()
	}
	p.Use(r)
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

func newFilsStore(dir string, fileTimeout time.Duration, enableMetrics bool) (filestore.FileStore, string, func() error, error) {
	const timeoutCheckInterval = 15 * time.Second
	var cleanUp func() error

	var fs filestore.FileStore
	if dir == "" {
		if runtime.GOOS == "linux" {
			dir = "/dev/shm"
		} else {
			dir = os.TempDir()
		}
		var err error
		dir, err = os.MkdirTemp(dir, "executorserver")
		if err != nil {
			return nil, "", cleanUp, err
		}
		cleanUp = func() error {
			return os.RemoveAll(dir)
		}
	}
	os.MkdirAll(dir, 0755)
	fs = filestore.NewFileLocalStore(dir)
	if enableMetrics {
		fs = newMetricsFileStore(fs)
	}
	if fileTimeout > 0 {
		fs = filestore.NewTimeout(fs, fileTimeout, timeoutCheckInterval)
	}
	return fs, dir, cleanUp, nil
}

func newEnvBuilder(conf *config.Config) pool.EnvBuilder {
	b, err := env.NewBuilder(env.Config{
		ContainerInitPath:  conf.ContainerInitPath,
		MountConf:          conf.MountConf,
		TmpFsParam:         conf.TmpFsParam,
		NetShare:           conf.NetShare,
		CgroupPrefix:       conf.CgroupPrefix,
		Cpuset:             conf.Cpuset,
		ContainerCredStart: conf.ContainerCredStart,
		EnableCPURate:      conf.EnableCPURate,
		CPUCfsPeriod:       conf.CPUCfsPeriod,
		SeccompConf:        conf.SeccompConf,
		Logger:             logger.Sugar(),
	})
	if err != nil {
		log.Fatalln("create environment builder failed", err)
	}
	if conf.EnableMetrics {
		b = &metriceEnvBuilder{b}
	}
	return b
}

func newEnvPool(b pool.EnvBuilder, enableMetrics bool) worker.EnvironmentPool {
	p := pool.NewPool(b)
	if enableMetrics {
		p = &metricsEnvPool{p}
	}
	return p
}

func newWorker(conf *config.Config, envPool worker.EnvironmentPool, fs filestore.FileStore) worker.Worker {
	return worker.New(worker.Config{
		FileStore:             fs,
		EnvironmentPool:       envPool,
		Parallelism:           conf.Parallelism,
		WorkDir:               conf.Dir,
		TimeLimitTickInterval: conf.TimeLimitCheckerInterval,
		ExtraMemoryLimit:      *conf.ExtraMemoryLimit,
		OutputLimit:           *conf.OutputLimit,
		CopyOutLimit:          *conf.CopyOutLimit,
		ExecObserver:          execObserve,
	})
}

func newForceGCWorker(conf *config.Config) {
	go func() {
		ticker := time.NewTicker(conf.ForceGCInterval)
		for {
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			if mem.HeapInuse > uint64(*conf.ForceGCTarget) {
				logger.Sugar().Infof("Force GC as heap_in_use(%v) > target(%v)",
					envexec.Size(mem.HeapInuse), *conf.ForceGCTarget)
				runtime.GC()
				debug.FreeOSMemory()
			}
			<-ticker.C
		}
	}()
}

func handleVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"buildVersion":    version.Version,
		"goVersion":       runtime.Version(),
		"platform":        runtime.GOARCH,
		"os":              runtime.GOOS,
		"copyOutOptional": true,
		"pipeProxy":       true,
	})
}

func generateHandleConfig(conf *config.Config) func(*gin.Context) {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"copyOutOptional": true,
			"pipeProxy":       true,
			"fileStorePath":   conf.Dir,
		})
	}
}
