package linuxcontainer

import (
	"syscall"

	"github.com/criyle/go-judge/env/pool"
)

// Config specifies configuration to build environment builder
type Config struct {
	Builder    EnvironmentBuilder
	CgroupPool CgroupPool
	WorkDir    string
	Seccomp    []syscall.SockFilter
	CPURate    bool
	CgroupFd   bool // whether to enable cgroup fd with clone3, kernel >= 5.7
}

type environmentBuilder struct {
	builder EnvironmentBuilder
	cgPool  CgroupPool
	workDir string
	seccomp []syscall.SockFilter
	cpuRate bool
	cgFd    bool
}

// NewEnvBuilder creates builder for linux container pools
func NewEnvBuilder(c Config) pool.EnvBuilder {
	return &environmentBuilder{
		builder: c.Builder,
		cgPool:  c.CgroupPool,
		workDir: c.WorkDir,
		seccomp: c.Seccomp,
		cpuRate: c.CPURate,
		cgFd:    c.CgroupFd,
	}
}

// Build creates linux container
func (b *environmentBuilder) Build() (pool.Environment, error) {
	m, err := b.builder.Build()
	if err != nil {
		return nil, err
	}
	return &environ{
		Environment: m,
		cgPool:      b.cgPool,
		workDir:     b.workDir,
		cpuRate:     b.cpuRate,
		seccomp:     b.seccomp,
		cgFd:        b.cgFd,
	}, nil
}
