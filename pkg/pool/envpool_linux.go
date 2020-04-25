package pool

import (
	"fmt"
	"sync"
	"syscall"

	"github.com/criyle/go-judge/pkg/envexec"
	"github.com/criyle/go-sandbox/container"
)

// EnvPool implements container environment pool
type EnvPool struct {
	builder EnvironmentBuilder
	cgPool  CgroupPool

	env []*Environment
	mu  sync.Mutex
}

// NewEnvPool creats new EnvPool with a builder
func NewEnvPool(builder EnvironmentBuilder, cgPool CgroupPool) *EnvPool {
	return &EnvPool{
		builder: builder,
		cgPool:  cgPool,
	}
}

// Get gets environment from pool, if pool is empty, creates new environment
func (p *EnvPool) Get() (envexec.Environment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.env) > 0 {
		rt := p.env[len(p.env)-1]
		p.env = p.env[:len(p.env)-1]
		return rt, nil
	}
	m, err := p.builder.Build()
	if err != nil {
		return nil, err
	}
	wd, err := m.Open([]container.OpenCmd{{
		Path: "/w",
		Flag: syscall.O_CLOEXEC | syscall.O_DIRECTORY,
		Perm: 0777,
	}})
	if err != nil {
		return nil, fmt.Errorf("container: failed to prepare work directory")
	}
	return &Environment{
		Environment: m,
		cgPool:      p.cgPool,
		wd:          wd[0],
	}, nil
}

// Put puts environment to the pool with reset the environment
func (p *EnvPool) Put(e envexec.Environment) {
	env, _ := e.(*Environment)
	env.Reset()

	p.mu.Lock()
	defer p.mu.Unlock()

	p.env = append(p.env, env)
}

// Destroy destory an environment
func (p *EnvPool) Destroy(e envexec.Environment) {
	env, _ := e.(*Environment)
	env.Destroy()
}

// Release clears the pool
func (p *EnvPool) Release() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, e := range p.env {
		p.Destroy(e)
	}
}

// Shutdown indicates a shutdown event, not implemented yes
func (p *EnvPool) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, e := range p.env {
		p.Destroy(e)
	}
}
