package pool

import (
	"fmt"
	"os"
	"sync"
	"syscall"

	"github.com/criyle/go-judge/pkg/envexec"
	"github.com/criyle/go-sandbox/container"
)

// Environment defines interface to access container resources
type Environment struct {
	container.Environment
	wd *os.File // container work dir
}

// EnvPool implements container environment pool
type EnvPool struct {
	builder EnvironmentBuilder

	env []envexec.Environment
	mu  sync.Mutex
}

// NewEnvPool creats new EnvPool with a builder
func NewEnvPool(builder EnvironmentBuilder) *EnvPool {
	return &EnvPool{
		builder: builder,
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
	return &Environment{Environment: m, wd: wd[0]}, nil
}

// Put puts environment to the pool with reset the environment
func (p *EnvPool) Put(env envexec.Environment) {
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

// WorkDir returns opened work directory, should not close after
func (c *Environment) WorkDir() *os.File {
	c.wd.Seek(0, 0)
	return c.wd
}

// OpenAtWorkDir opens file relative to work directory
func (c *Environment) OpenAtWorkDir(path string, flags int, perm os.FileMode) (*os.File, error) {
	fd, err := syscall.Openat(int(c.wd.Fd()), path, flags|syscall.O_CLOEXEC, uint32(perm))
	if err != nil {
		return nil, fmt.Errorf("openAtWorkDir: %v", err)
	}
	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		return nil, fmt.Errorf("openAtWorkDir: failed to NewFile")
	}
	return f, nil
}
