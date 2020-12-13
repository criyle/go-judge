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
	"strings"
	"time"

	"github.com/criyle/go-judge/cmd/executorserver/config"
	"github.com/criyle/go-judge/env"
	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/pb"
	"github.com/criyle/go-judge/pkg/envexec"
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

var logger *zap.Logger

func main() {
	var conf config.Config
	if err := conf.Load(); err != nil {
		if err == flag.ErrHelp {
			return
		}
		log.Fatalln("load config failed", err)
	}

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

	logger.Sugar().Infof("config loaded: %+v", conf)

	// Init environment pool
	fs := newFilsStore(conf.Dir)
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
		Logger:             logger.Sugar(),
	})
	if err != nil {
		log.Fatalln("create environment builder failed", err)
	}
	envPool := pool.NewPool(b)
	if conf.PreFork > 0 {
		logger.Sugar().Info("create ", conf.PreFork, " prefork containers")
		m := make([]envexec.Environment, 0, conf.PreFork)
		for i := 0; i < conf.PreFork; i++ {
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
	work := worker.New(worker.Config{
		FileStore:             fs,
		EnvironmentPool:       envPool,
		Parallelism:           conf.Parallelism,
		WorkDir:               conf.Dir,
		TimeLimitTickInterval: conf.TimeLimitCheckerInterval,
		ExtraMemoryLimit:      *conf.ExtraMemoryLimit,
		OutputLimit:           *conf.OutputLimit,
	})
	work.Start()
	logger.Sugar().Infof("Starting worker with parallelism=%d, workdir=%s, timeLimitCheckInterval=%v",
		conf.Parallelism, conf.Dir, conf.TimeLimitCheckerInterval)

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

	// Version handle
	r.GET("/version", handleVersion)

	// Add auth token
	if conf.AuthToken != "" {
		r.Use(tokenAuth(conf.AuthToken))
		logger.Sugar().Info("Attach token auth with token:", conf.AuthToken)
	}

	// File Handles
	fh := &fileHandle{fs: fs}
	r.GET("/file", fh.fileGet)
	r.POST("/file", fh.filePost)
	r.GET("/file/:fid", fh.fileIDGet)
	r.DELETE("/file/:fid", fh.fileIDDelete)

	// Run Handle
	rh := &cmdHandle{worker: work, srcPrefix: conf.SrcPrefix}
	r.POST("/run", rh.handleRun)

	// WebSocket Handle
	wh := &wsHandle{worker: work, srcPrefix: conf.SrcPrefix}
	r.GET("/ws", wh.handleWS)

	// pprof
	if conf.EnableDebug {
		ginpprof.Register(r)
	}

	// gRPC server
	var grpcServer *grpc.Server
	if conf.EnableGRPC {
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
		pb.RegisterExecutorServer(grpcServer, &execServer{
			fs:        fs,
			worker:    work,
			srcPrefix: conf.SrcPrefix,
		})
		grpc_prometheus.Register(grpcServer)
		grpc_prometheus.EnableHandlingTimeHistogram()

		lis, err := net.Listen("tcp", conf.GRPCAddr)
		if err != nil {
			log.Fatalln(err)
		}
		go func() {
			logger.Sugar().Info("Starting gRPC server at ", conf.GRPCAddr)
			logger.Sugar().Info("gRPC serve finished: ", grpcServer.Serve(lis))
		}()
	}

	srv := http.Server{
		Addr:    conf.HTTPAddr,
		Handler: r,
	}

	go func() {
		logger.Sugar().Info("Starting http server at ", conf.HTTPAddr)
		logger.Sugar().Info("Http serve finished: ", srv.ListenAndServe())
	}()

	// Graceful shutdown...
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig

	logger.Sugar().Info("Shutting Down...")

	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*3)
	defer cancel()

	var eg errgroup.Group
	eg.Go(func() error {
		logger.Sugar().Info("Http server shutdown")
		return srv.Shutdown(ctx)
	})

	eg.Go(func() error {
		work.Shutdown()
		logger.Sugar().Info("Worker shutdown")
		return nil
	})

	if grpcServer != nil {
		eg.Go(func() error {
			grpcServer.GracefulStop()
			logger.Sugar().Info("GRPC server shutdown")
			return nil
		})
	}

	go func() {
		logger.Sugar().Info("Shutdown Finished: ", eg.Wait())
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
