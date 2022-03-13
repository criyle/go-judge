package linuxcontainer

import (
	"fmt"
	"syscall"

	"github.com/criyle/go-judge/env/pool"
	"github.com/criyle/go-sandbox/container"
)

// Config specifies configuration to build environment builder
type Config struct {
	Builder    EnvironmentBuilder
	CgroupPool CgroupPool
	WorkDir    string
	Seccomp    []syscall.SockFilter
	Cpuset     string
	CPURate    bool
}

type environmentBuilder struct {
	builder EnvironmentBuilder
	cgPool  CgroupPool
	workDir string
	seccomp []syscall.SockFilter
	cpuset  string
	cpuRate bool
}

// NewEnvBuilder creates builder for linux container pools
func NewEnvBuilder(c Config) pool.EnvBuilder {
	return &environmentBuilder{
		builder: c.Builder,
		cgPool:  c.CgroupPool,
		workDir: c.WorkDir,
		seccomp: c.Seccomp,
		cpuset:  c.Cpuset,
		cpuRate: c.CPURate,
	}
}

// Build creates linux container
func (b *environmentBuilder) Build() (pool.Environment, error) {
	m, err := b.builder.Build()
	if err != nil {
		return nil, err
	}
	wd, err := m.Open([]container.OpenCmd{{
		Path: b.workDir,
		Flag: syscall.O_CLOEXEC | syscall.O_DIRECTORY,
		Perm: 0777,
	}})
	if err != nil {
		return nil, fmt.Errorf("container: failed to prepare work directory")
	}
	return &environ{
		Environment: m,
		cgPool:      b.cgPool,
		wd:          wd[0],
		workDir:     b.workDir,
		cpuset:      b.cpuset,
		cpuRate:     b.cpuRate,
		seccomp:     b.seccomp,
	}, nil
}
