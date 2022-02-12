package linuxcontainer

import (
	"sync"
	"time"

	"github.com/criyle/go-judge/envexec"
)

// Cgroup defines interface to limit and monitor resources consumption of a process
type Cgroup interface {
	SetCpuset(string) error
	SetMemoryLimit(envexec.Size) error
	SetProcLimit(uint64) error
	SetCPURate(uint64) error // 1000 as 1

	CPUUsage() (time.Duration, error)
	CurrentMemory() (envexec.Size, error)
	MaxMemory() (envexec.Size, error)

	AddProc(int) error
	Reset() error
	Destroy() error
}

// CgroupPool implements pool of Cgroup
type CgroupPool interface {
	Get() (Cgroup, error)
	Put(Cgroup)
}

// CgroupListPool implements cgroup pool
type CgroupListPool struct {
	builder   CgroupBuilder
	cfsPeriod time.Duration

	cgs []Cgroup
	mu  sync.Mutex
}

// NewCgroupListPool creates new cgroup pool
func NewCgroupListPool(builder CgroupBuilder, cfsPeriod time.Duration) CgroupPool {
	return &CgroupListPool{builder: builder, cfsPeriod: cfsPeriod}
}

// Get gets cgroup from pool, if pool is empty, creates new one
func (w *CgroupListPool) Get() (Cgroup, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.cgs) > 0 {
		rt := w.cgs[len(w.cgs)-1]
		w.cgs = w.cgs[:len(w.cgs)-1]
		return rt, nil
	}

	cg, err := w.builder.Random("")
	if err != nil {
		return nil, err
	}
	return &wCgroup{cg: cg, cfsPeriod: w.cfsPeriod}, nil
}

// Put puts cgroup into the pool
func (w *CgroupListPool) Put(c Cgroup) {
	w.mu.Lock()
	defer w.mu.Unlock()

	c.Reset()
	w.cgs = append(w.cgs, c)
}

// Shutdown destroy all cgroup
func (w *CgroupListPool) Shutdown() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, c := range w.cgs {
		c.Destroy()
	}
}
