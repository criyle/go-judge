package env

import "time"

// Logger defines logger to print logs
type Logger interface {
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
}

// Config defines parameters to create environment builder
type Config struct {
	ContainerInitPath  string
	TmpFsParam         string
	NetShare           bool
	MountConf          string
	SeccompConf        string
	CgroupPrefix       string
	Cpuset             string
	ContainerCredStart int
	EnableCPURate      bool
	CPUCfsPeriod       time.Duration
	Logger
}
