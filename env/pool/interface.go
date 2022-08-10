package pool

import (
	"sync"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/worker"
)

// Environment defines envexec.Environment with destroy
type Environment interface {
	envexec.Environment
	Reset() error
	Destroy() error
}

// EnvBuilder defines the abstract builder for container environment
type EnvBuilder interface {
	Build() (Environment, error)
}

type pool struct {
	builder EnvBuilder

	env []Environment
	mu  sync.Mutex
}

// NewPool returns a pool for EnvBuilder
func NewPool(builder EnvBuilder) worker.EnvironmentPool {
	return &pool{
		builder: builder,
	}
}

func (p *pool) Get() (envexec.Environment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.env) > 0 {
		rt := p.env[len(p.env)-1]
		p.env = p.env[:len(p.env)-1]
		return rt, nil
	}
	return p.builder.Build()
}

func (p *pool) Put(env envexec.Environment) {
	e, ok := env.(Environment)
	if !ok {
		panic("invalid environment put")
	}
	// If contain died after execution, don't put it into pool
	if err := e.Reset(); err != nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.env = append(p.env, e)
}
