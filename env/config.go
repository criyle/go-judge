package env

import "time"

// Config defines parameters to create environment builder
type Config struct {
	ContainerInitPath  string
	TmpFsParam         string
	NetShare           bool
	MountConf          string
	SeccompConf        string
	NoSeccomp          bool
	CgroupPrefix       string
	ContainerCredStart int
	EnableCPURate      bool
	CPUCfsPeriod       time.Duration
	NoFallback         bool
}
