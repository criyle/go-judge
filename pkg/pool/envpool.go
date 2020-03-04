package pool

import (
	"sync"

	"github.com/criyle/go-sandbox/container"
)

// EnvPool implements container environment pool
type EnvPool struct {
	builder EnvironmentBuilder

	env []container.Environment
	mu  sync.Mutex
}

// NewEnvPool creats new EnvPool with a builder
func NewEnvPool(builder EnvironmentBuilder) *EnvPool {
	return &EnvPool{
		builder: builder,
	}
}

// Get gets environment from pool, if pool is empty, creates new environment
func (p *EnvPool) Get() (container.Environment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.env) > 0 {
		rt := p.env[len(p.env)-1]
		p.env = p.env[:len(p.env)-1]
		return rt, nil
	}
	return p.builder.Build()
}

// Put puts environment to the pool with reset the environment
func (p *EnvPool) Put(env container.Environment) {
	env.Reset()

	p.mu.Lock()
	defer p.mu.Unlock()

	p.env = append(p.env, env)
}

// Destroy destory an environment
func (p *EnvPool) Destroy(env container.Environment) {
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
