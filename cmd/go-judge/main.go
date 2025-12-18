// Command go-judge will starts a http server that receives command to run
// programs inside a sandbox.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/criyle/go-judge/cmd/go-judge/config"
	grpcexecutor "github.com/criyle/go-judge/cmd/go-judge/grpc_executor"
	restexecutor "github.com/criyle/go-judge/cmd/go-judge/rest_executor"
	"github.com/criyle/go-judge/cmd/go-judge/version"
	wsexecutor "github.com/criyle/go-judge/cmd/go-judge/ws_executor"
	"github.com/criyle/go-judge/env"
	"github.com/criyle/go-judge/env/pool"
	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/pb"
	"github.com/criyle/go-judge/worker"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	grpc_logging "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	ginprometheus "github.com/zsais/go-gin-prometheus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zapgrpc"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/status"
)

var logger *zap.Logger

func main() {
	conf := loadConf()
	if conf.Version {
		fmt.Println(version.Version)
		return
	}
	initLogger(conf)
	defer logger.Sync()
	if ce := logger.Check(zap.InfoLevel, "Config loaded"); ce != nil {
		ce.Write(zap.String("config", fmt.Sprintf("%+v", conf)))
	}
	warnIfNotLinux()

	// Init environment pool
	fs, fsCleanUp := newFileStore(conf)
	b, builderParam := newEnvBuilder(conf)
	envPool := newEnvPool(b, conf.EnableMetrics)
	prefork(envPool, conf.PreFork)
	work := newWorker(conf, envPool, fs)
	work.Start()
	logger.Info("Worker stated ",
		zap.Int("parallelism", conf.Parallelism),
		zap.String("dir", conf.Dir),
		zap.Duration("timeLimitCheckInterval", conf.TimeLimitCheckerInterval))
	initCgroupMetrics(conf, builderParam)

	servers := []initFunc{
		cleanUpWorker(work),
		cleanUpFs(fsCleanUp),
		initHTTPServer(conf, work, fs, builderParam),
		initMonitorHTTPServer(conf),
		initGRPCServer(conf, work, fs),
	}

	// Gracefully shutdown, with signal / HTTP server / gRPC server / Monitor HTTP server
	sig := make(chan os.Signal, 1+len(servers))

	// worker and fs clean up func
	stops := []stopFunc{}
	for _, s := range servers {
		start, stop := s()
		if start != nil {
			go func() {
				start()
				sig <- os.Interrupt
			}()
		}
		if stop != nil {
			stops = append(stops, stop)
		}
	}

	// background force GC worker
	newForceGCWorker(conf)

	// Graceful shutdown...
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
loop:
	for s := range sig {
		switch s {
		case syscall.SIGINT:
			break loop
		case syscall.SIGTERM:
			if isManagedByPM2() {
				logger.Info("running with PM2, received SIGTERM (from systemd), ignoring")
			} else {
				break loop
			}
		}
	}
	signal.Reset(syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Shutting Down...")

	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*3)
	defer cancel()

	var eg errgroup.Group
	for _, s := range stops {
		s := s
		eg.Go(func() error {
			return s(ctx)
		})
	}

	go func() {
		logger.Info("Shutdown Finished", zap.Error(eg.Wait()))
		cancel()
	}()
	<-ctx.Done()
}

func warnIfNotLinux() {
	if runtime.GOOS != "linux" {
		logger.Warn("Platform is not primarily supported", zap.String("GOOS", runtime.GOOS))
		logger.Warn("Please notice that the primary supporting platform is Linux")
		logger.Warn("Windows and macOS(darwin) support are only recommended in development environment")
	}
}

func loadConf() *config.Config {
	var conf config.Config
	if err := conf.Load(); err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		log.Fatalln("load config failed ", err)
	}
	return &conf
}

type (
	stopFunc func(ctx context.Context) error
	initFunc func() (start func(), cleanUp stopFunc)
)

func cleanUpWorker(work worker.Worker) initFunc {
	return func() (start func(), cleanUp stopFunc) {
		return nil, func(ctx context.Context) error {
			work.Shutdown()
			logger.Info("Worker shutdown")
			return nil
		}
	}
}

func cleanUpFs(fsCleanUp func() error) initFunc {
	return func() (start func(), cleanUp stopFunc) {
		if fsCleanUp == nil {
			return nil, nil
		}
		return nil, func(ctx context.Context) error {
			err := fsCleanUp()
			logger.Info("FileStore cleaned up")
			return err
		}
	}
}

func initHTTPServer(conf *config.Config, work worker.Worker, fs filestore.FileStore, builderParam map[string]any) initFunc {
	return func() (start func(), cleanUp stopFunc) {
		// Init http handle
		r := initHTTPMux(conf, work, fs, builderParam)
		srv := http.Server{
			Addr:    conf.HTTPAddr,
			Handler: r,
		}

		return func() {
				lis, err := newListener(conf.HTTPAddr)
				if err != nil {
					logger.Error("Http server listen failed", zap.Error(err))
					return
				}
				logger.Info("Starting http server", zap.String("addr", conf.HTTPAddr), zap.String("listener", printListener(lis)))
				if err := srv.Serve(lis); errors.Is(err, http.ErrServerClosed) {
					logger.Info("Http server stopped", zap.Error(err))
				} else {
					logger.Error("Http server stopped", zap.Error(err))
				}
			}, func(ctx context.Context) error {
				logger.Info("Http server shutting down")
				return srv.Shutdown(ctx)
			}
	}
}

func initMonitorHTTPServer(conf *config.Config) initFunc {
	return func() (start func(), cleanUp stopFunc) {
		// Init monitor HTTP server
		mr := initMonitorHTTPMux(conf)
		if mr == nil {
			return nil, nil
		}
		msrv := http.Server{
			Addr:    conf.MonitorAddr,
			Handler: mr,
		}
		return func() {
				lis, err := newListener(conf.MonitorAddr)
				if err != nil {
					logger.Error("Monitoring http listen failed", zap.Error(err))
					return
				}
				logger.Info("Starting monitoring http server", zap.String("addr", conf.MonitorAddr), zap.String("listener", printListener(lis)))
				logger.Info("Monitoring http server stopped", zap.Error(msrv.Serve(lis)))
			}, func(ctx context.Context) error {
				logger.Info("Monitoring http server shutdown")
				return msrv.Shutdown(ctx)
			}
	}
}

func initGRPCServer(conf *config.Config, work worker.Worker, fs filestore.FileStore) initFunc {
	return func() (start func(), cleanUp stopFunc) {
		if !conf.EnableGRPC {
			return nil, nil
		}
		// Init gRPC server
		esServer := grpcexecutor.New(work, fs, conf.SrcPrefix, logger)
		grpcServer := newGRPCServer(conf, esServer)

		return func() {
				lis, err := newListener(conf.GRPCAddr)
				if err != nil {
					logger.Error("gRPC listen failed: ", zap.Error(err))
					return
				}
				logger.Info("Starting gRPC server", zap.String("addr", conf.GRPCAddr), zap.String("listener", printListener(lis)))
				logger.Info("gRPC server stopped", zap.Error(grpcServer.Serve(lis)))
			}, func(ctx context.Context) error {
				grpcServer.GracefulStop()
				logger.Info("GRPC server shutdown")
				return nil
			}
	}
}

func initLogger(conf *config.Config) {
	if conf.Silent {
		logger = zap.NewNop()
		return
	}

	var err error
	if conf.Release {
		logger, err = zap.NewProduction()
	} else {
		config := zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		if !conf.EnableDebug {
			config.Level.SetLevel(zap.InfoLevel)
		}
		logger, err = config.Build()
	}
	if err != nil {
		log.Fatalln("init logger failed ", err)
	}
}

func prefork(envPool worker.EnvironmentPool, prefork int) {
	if prefork <= 0 {
		return
	}
	logger.Info("Create prefork containers", zap.Int("count", prefork))
	m := make([]envexec.Environment, 0, prefork)
	for i := 0; i < prefork; i++ {
		e, err := envPool.Get()
		if err != nil {
			log.Fatalln("prefork environment failed ", err)
		}
		m = append(m, e)
	}
	for _, e := range m {
		envPool.Put(e)
	}
}

func initHTTPMux(conf *config.Config, work worker.Worker, fs filestore.FileStore, builderParam map[string]any) http.Handler {
	var r *gin.Engine
	if conf.Release {
		gin.SetMode(gin.ReleaseMode)
	}
	r = gin.New()
	r.Use(ginzap.Ginzap(logger, "", false))
	r.Use(ginzap.RecoveryWithZap(logger, true))

	// Metrics Handle
	if conf.EnableMetrics {
		initGinMetrics(r)
	}

	// Version handle
	r.GET("/version", generateHandleVersion(conf, builderParam))

	// Config handle
	r.GET("/config", generateHandleConfig(conf, builderParam))

	// Add auth token
	if conf.AuthToken != "" {
		r.Use(tokenAuth(conf.AuthToken))
		logger.Info("Attach token auth", zap.String("token", conf.AuthToken))
	}

	// Rest Handle
	cmdHandle := restexecutor.NewCmdHandle(work, conf.SrcPrefix, logger)
	cmdHandle.Register(r)
	fileHandle := restexecutor.NewFileHandle(fs)
	fileHandle.Register(r)

	// WebSocket Handle
	wsHandle := wsexecutor.New(work, conf.SrcPrefix, logger)
	wsHandle.Register(r)

	return r
}

func initMonitorHTTPMux(conf *config.Config) http.Handler {
	if !conf.EnableMetrics && !conf.EnableDebug {
		return nil
	}
	mux := http.NewServeMux()
	if conf.EnableMetrics {
		mux.Handle("/metrics", promhttp.Handler())
	}
	if conf.EnableDebug {
		initDebugRoute(mux)
	}
	return mux
}

func initDebugRoute(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}

func InterceptorLogger(l *zap.Logger) grpc_logging.Logger {
	return grpc_logging.LoggerFunc(func(ctx context.Context, lvl grpc_logging.Level, msg string, fields ...any) {
		f := make([]zap.Field, 0, len(fields)/2)

		for i := 0; i < len(fields); i += 2 {
			key := fields[i]
			value := fields[i+1]

			switch v := value.(type) {
			case string:
				f = append(f, zap.String(key.(string), v))
			case int:
				f = append(f, zap.Int(key.(string), v))
			case bool:
				f = append(f, zap.Bool(key.(string), v))
			default:
				f = append(f, zap.Any(key.(string), v))
			}
		}

		logger := l.WithOptions(zap.AddCallerSkip(1)).With(f...)

		switch lvl {
		case grpc_logging.LevelDebug:
			logger.Debug(msg)
		case grpc_logging.LevelInfo:
			logger.Info(msg)
		case grpc_logging.LevelWarn:
			logger.Warn(msg)
		case grpc_logging.LevelError:
			logger.Error(msg)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}

func newGRPCServer(conf *config.Config, esServer pb.ExecutorServer) *grpc.Server {
	prom := grpc_prometheus.NewServerMetrics(grpc_prometheus.WithServerHandlingTimeHistogram())
	grpclog.SetLoggerV2(zapgrpc.NewLogger(logger))
	streamMiddleware := []grpc.StreamServerInterceptor{
		prom.StreamServerInterceptor(),
		grpc_logging.StreamServerInterceptor(InterceptorLogger(logger)),
		grpc_recovery.StreamServerInterceptor(),
	}
	unaryMiddleware := []grpc.UnaryServerInterceptor{
		prom.UnaryServerInterceptor(),
		grpc_logging.UnaryServerInterceptor(InterceptorLogger(logger)),
		grpc_recovery.UnaryServerInterceptor(),
	}
	if conf.AuthToken != "" {
		authFunc := grpcTokenAuth(conf.AuthToken)
		streamMiddleware = append(streamMiddleware, grpc_auth.StreamServerInterceptor(authFunc))
		unaryMiddleware = append(unaryMiddleware, grpc_auth.UnaryServerInterceptor(authFunc))
	}
	grpcServer := grpc.NewServer(
		grpc.ChainStreamInterceptor(streamMiddleware...),
		grpc.ChainUnaryInterceptor(unaryMiddleware...),
		grpc.MaxRecvMsgSize(int(conf.GRPCMsgSize.Byte())),
	)
	pb.RegisterExecutorServer(grpcServer, esServer)
	prometheus.MustRegister(prom)
	return grpcServer
}

func initGinMetrics(r *gin.Engine) {
	p := ginprometheus.NewWithConfig(ginprometheus.Config{
		Subsystem:          "gin",
		DisableBodyReading: true,
	})
	p.ReqCntURLLabelMappingFn = func(c *gin.Context) string {
		return c.FullPath()
	}
	r.Use(p.HandlerFunc())
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

func newFileStore(conf *config.Config) (filestore.FileStore, func() error) {
	const timeoutCheckInterval = 15 * time.Second
	var cleanUp func() error

	var fs filestore.FileStore
	if conf.Dir == "" {
		if runtime.GOOS == "linux" {
			conf.Dir = "/dev/shm"
		} else {
			conf.Dir = os.TempDir()
		}
		var err error
		conf.Dir = filepath.Join(conf.Dir, "go-judge")
		err = os.Mkdir(conf.Dir, os.ModePerm)
		if err != nil && !errors.Is(err, os.ErrExist) {
			logger.Fatal("Failed to create file store default dir", zap.Error(err))
		}
		cleanUp = func() error {
			return os.RemoveAll(conf.Dir)
		}
	}
	os.MkdirAll(conf.Dir, 0o755)
	fs = filestore.NewFileLocalStore(conf.Dir)
	if conf.EnableMetrics {
		fs = newMetricsFileStore(fs)
	}
	if conf.FileTimeout > 0 {
		fs = filestore.NewTimeout(fs, conf.FileTimeout, timeoutCheckInterval)
	}
	return fs, cleanUp
}

func newEnvBuilder(conf *config.Config) (pool.EnvBuilder, map[string]any) {
	b, param, err := env.NewBuilder(env.Config{
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
		NoFallback:         conf.NoFallback,
	}, logger)
	if err != nil {
		logger.Fatal("create environment builder failed ", zap.Error(err))
	}
	if conf.EnableMetrics {
		b = &metricsEnvBuilder{b}
	}
	return b, param
}

func newEnvPool(b pool.EnvBuilder, enableMetrics bool) worker.EnvironmentPool {
	p := pool.NewPool(b)
	if enableMetrics {
		p = &metricsEnvPool{p}
	}
	return p
}

func newWorker(conf *config.Config, envPool worker.EnvironmentPool, fs filestore.FileStore) worker.Worker {
	w := worker.New(worker.Config{
		FileStore:             fs,
		EnvironmentPool:       envPool,
		Parallelism:           conf.Parallelism,
		WorkDir:               conf.Dir,
		TimeLimitTickInterval: conf.TimeLimitCheckerInterval,
		ExtraMemoryLimit:      *conf.ExtraMemoryLimit,
		OutputLimit:           *conf.OutputLimit,
		CopyOutLimit:          *conf.CopyOutLimit,
		OpenFileLimit:         uint64(conf.OpenFileLimit),
		ExecObserver:          execObserve,
	})
	if conf.EnableMetrics {
		w = newMetricsWorker(w)
	}
	return w
}

func newForceGCWorker(conf *config.Config) {
	go func() {
		ticker := time.NewTicker(conf.ForceGCInterval)
		for {
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			if mem.HeapInuse > uint64(*conf.ForceGCTarget) {
				logger.Info("Force GC as heap_in_use > target",
					zap.Stringer("heap_in_use", envexec.Size(mem.HeapInuse)),
					zap.Stringer("target", *conf.ForceGCTarget))
				runtime.GC()
				debug.FreeOSMemory()
			}
			<-ticker.C
		}
	}()
}

func generateHandleVersion(_ *config.Config, _ map[string]any) func(*gin.Context) {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"buildVersion":      version.Version,
			"goVersion":         runtime.Version(),
			"platform":          runtime.GOARCH,
			"os":                runtime.GOOS,
			"copyOutOptional":   true,
			"pipeProxy":         true,
			"symlink":           true,
			"addressSpaceLimit": true,
			"stream":            true,
			"procPeak":          true,
			"copyOutTruncate":   true,
			"pipeProxyZeroCopy": true,
		})
	}
}

func generateHandleConfig(conf *config.Config, builderParam map[string]any) func(*gin.Context) {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"copyOutOptional":   true,
			"pipeProxy":         true,
			"symlink":           true,
			"addressSpaceLimit": true,
			"stream":            true,
			"procPeak":          true,
			"copyOutTruncate":   true,
			"pipeProxyZeroCopy": true,
			"fileStorePath":     conf.Dir,
			"runnerConfig":      builderParam,
		})
	}
}

func isManagedByPM2() bool {
	// List of environment variables that pm2 typically sets.
	pm2EnvVars := []string{
		"PM2_HOME",
		"PM2_JSON_PROCESSING",
		"NODE_APP_INSTANCE",
	}
	for _, v := range pm2EnvVars {
		if os.Getenv(v) != "" {
			return true
		}
	}
	return false
}
