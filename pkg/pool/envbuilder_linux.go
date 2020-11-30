package pool

import (
	"fmt"
	"syscall"

	"github.com/criyle/go-sandbox/container"
)

type environmentBuilder struct {
	builder EnvironmentBuilder
	cgPool  CgroupPool
	workDir string
}

// NewEnvBuilder creates builder for linux container pools
func NewEnvBuilder(builder EnvironmentBuilder, cgPool CgroupPool, workDir string) EnvBuilder {
	return &environmentBuilder{
		builder: builder,
		cgPool:  cgPool,
		workDir: workDir,
	}
}

// Build creates linux container
func (b *environmentBuilder) Build() (Environment, error) {
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
	}, nil
}
