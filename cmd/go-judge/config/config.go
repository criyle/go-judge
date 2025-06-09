package config

import (
	"os"
	"runtime"
	"time"

	"github.com/criyle/go-judge/envexec"
	"github.com/koding/multiconfig"
)

// Config defines go judge server configuration
type Config struct {
	// container
	ContainerInitPath  string `flagUsage:"container init path"`
	PreFork            int    `flagUsage:"control # of the prefork workers" default:"1"`
	TmpFsParam         string `flagUsage:"tmpfs mount data (only for default mount with no mount.yaml)" default:"size=128m,nr_inodes=4k"`
	NetShare           bool   `flagUsage:"share net namespace with host"`
	MountConf          string `flagUsage:"specifies mount configuration file" default:"mount.yaml"`
	SeccompConf        string `flagUsage:"specifies seccomp filter" default:"seccomp.yaml"`
	Parallelism        int    `flagUsage:"control the # of concurrency execution (default equal to number of cpu)"`
	CgroupPrefix       string `flagUsage:"control cgroup prefix" default:"gojudge"`
	ContainerCredStart int    `flagUsage:"control the start uid&gid for container (0 uses unprivileged root)" default:"0"`
	NoFallback         bool   `flagUsage:"exit if fallback to rlimit / rusage mode"`

	// file store
	SrcPrefix []string `flagUsage:"specifies directory prefix for source type copyin (example: -src-prefix=/home,/usr)"`
	Dir       string   `flagUsage:"specifies directory to store file upload / download (in memory by default)"`

	// runner limit
	TimeLimitCheckerInterval time.Duration `flagUsage:"specifies time limit checker interval" default:"100ms"`
	ExtraMemoryLimit         *envexec.Size `flagUsage:"specifies extra memory buffer for check memory limit" default:"16k"`
	OutputLimit              *envexec.Size `flagUsage:"specifies POSIX rlimit for output for each command" default:"256m"`
	CopyOutLimit             *envexec.Size `flagUsage:"specifies default file copy out max" default:"256m"`
	OpenFileLimit            int           `flagUsage:"specifies max open file count" default:"256"`
	Cpuset                   string        `flagUsage:"control the usage of cpuset for all container process"`
	EnableCPURate            bool          `flagUsage:"enable cpu cgroup rate control"`
	CPUCfsPeriod             time.Duration `flagUsage:"set cpu.cfs_period" default:"100ms"`
	FileTimeout              time.Duration `flagUsage:"specified timeout for filestore files"`

	// server config
	HTTPAddr      string        `flagUsage:"specifies the http binding address"`
	EnableGRPC    bool          `flagUsage:"enable gRPC endpoint"`
	GRPCAddr      string        `flagUsage:"specifies the grpc binding address"`
	MonitorAddr   string        `flagUsage:"specifies the metrics binding address"`
	AuthToken     string        `flagUsage:"bearer token auth for REST / gRPC"`
	GRPCMsgSize   *envexec.Size `flagUsage:"message size limit for gRPC message" default:"64m"`
	EnableDebug   bool          `flagUsage:"enable debug endpoint"`
	EnableMetrics bool          `flagUsage:"enable prometheus metrics endpoint"`

	// logger config
	Release bool `flagUsage:"release level of logs"`
	Silent  bool `flagUsage:"do not print logs"`

	// fix for high memory usage
	ForceGCTarget   *envexec.Size `flagUsage:"specifies force GC trigger heap size" default:"20m"`
	ForceGCInterval time.Duration `flagUsage:"specifies force GC trigger interval" default:"5s"`

	// show version and exit
	Version bool `flagUsage:"show version and exit"`
}

// Load loads config from flag & environment variables
func (c *Config) Load() error {
	cl := multiconfig.MultiLoader(
		&multiconfig.TagLoader{},
		&multiconfig.EnvironmentLoader{
			Prefix:    "ES",
			CamelCase: true,
		},
		&multiconfig.FlagLoader{
			CamelCase: true,
			EnvPrefix: "ES",
		},
	)
	if os.Getpid() == 1 {
		c.Release = true
		c.HTTPAddr = ":5050"
		c.GRPCAddr = ":5051"
		c.MonitorAddr = ":5052"
	} else {
		c.HTTPAddr = "localhost:5050"
		c.GRPCAddr = "localhost:5051"
		c.MonitorAddr = "localhost:5052"
	}
	if c.Parallelism <= 0 {
		c.Parallelism = runtime.NumCPU()
	}
	return cl.Load(c)
}
