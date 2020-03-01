package runner

import (
	"sync"
	"time"

	"github.com/criyle/go-judge/pkg/runner"
	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/pkg/cgroup"
	stypes "github.com/criyle/go-sandbox/types"
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

type wCgroup cgroup.CGroup

func (c *wCgroup) SetMemoryLimit(s stypes.Size) error {
	return (*cgroup.CGroup)(c).SetMemoryLimitInBytes(uint64(s))
}

func (c *wCgroup) SetProcLimit(l uint64) error {
	return (*cgroup.CGroup)(c).SetPidsMax(l)
}

func (c *wCgroup) CPUUsage() (time.Duration, error) {
	t, err := (*cgroup.CGroup)(c).CpuacctUsage()
	return time.Duration(t), err
}

func (c *wCgroup) MemoryUsage() (stypes.Size, error) {
	s, err := (*cgroup.CGroup)(c).MemoryMaxUsageInBytes()
	if err != nil {
		return 0, err
	}
	cache, err := (*cgroup.CGroup)(c).FindMemoryStatProperty("cache")
	if err != nil {
		return 0, err
	}
	return stypes.Size(s - cache), err
}

func (c *wCgroup) AddProc(pid int) error {
	return (*cgroup.CGroup)(c).AddProc(pid)
}

func (c *wCgroup) Reset() error {
	if err := (*cgroup.CGroup)(c).SetCpuacctUsage(0); err != nil {
		return err
	}
	if err := (*cgroup.CGroup)(c).SetMemoryMaxUsageInBytes(0); err != nil {
		return err
	}
	return nil
}

func (c *wCgroup) Destory() error {
	return (*cgroup.CGroup)(c).Destroy()
}

type wCgroupPool struct {
	builder CgroupBuilder

	cgs []runner.Cgroup
	mu  sync.Mutex
}

func newCgroupPool(builder CgroupBuilder) *wCgroupPool {
	return &wCgroupPool{builder: builder}
}

func (w *wCgroupPool) Get() (runner.Cgroup, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.cgs) > 0 {
		rt := w.cgs[len(w.cgs)-1]
		w.cgs = w.cgs[:len(w.cgs)-1]
		return rt, nil
	}

	cg, err := w.builder.Build()
	if err != nil {
		return nil, err
	}
	return (*wCgroup)(cg), nil
}

func (w *wCgroupPool) Put(c runner.Cgroup) {
	w.mu.Lock()
	defer w.mu.Unlock()

	c.Reset()
	w.cgs = append(w.cgs, c)
}

func (w *wCgroupPool) Shutdown() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, c := range w.cgs {
		c.Destory()
	}
}
