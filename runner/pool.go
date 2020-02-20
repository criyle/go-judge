package runner

import (
	"sync"

	"github.com/criyle/go-sandbox/container"
)

type pool struct {
	builder EnvironmentBuilder

	env []container.Environment
	mu  sync.Mutex
}

func newPool(builder EnvironmentBuilder) *pool {
	return &pool{
		builder: builder,
	}
}

func (p *pool) Get() (container.Environment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.env) > 0 {
		rt := p.env[len(p.env)-1]
		p.env = p.env[:len(p.env)-1]
		return rt, nil
	}
	return p.builder.Build()
}

func (p *pool) Put(env container.Environment) {
	env.Reset()

	p.mu.Lock()
	defer p.mu.Unlock()

	p.env = append(p.env, env)
}

func (p *pool) Destroy(env container.Environment) {
	env.Destroy()
}

func (p *pool) Release() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, e := range p.env {
		p.Destroy(e)
	}
}

func (p *pool) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, e := range p.env {
		p.Destroy(e)
	}
}
