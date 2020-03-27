package pool

import (
	"sync"

	"github.com/criyle/go-judge/pkg/envexec"
)

// CgroupPool implements cgroup pool
type CgroupPool struct {
	builder CgroupBuilder

	cgs []envexec.Cgroup
	mu  sync.Mutex
}

// NewCgroupPool creates new cgroup pool
func NewCgroupPool(builder CgroupBuilder) *CgroupPool {
	return &CgroupPool{builder: builder}
}

// Get gets cgroup from pool, if pool is empty, creates new one
func (w *CgroupPool) Get() (envexec.Cgroup, error) {
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

// Put puts cgroup into the pool
func (w *CgroupPool) Put(c envexec.Cgroup) {
	w.mu.Lock()
	defer w.mu.Unlock()

	c.Reset()
	w.cgs = append(w.cgs, c)
}

// Shutdown destroy all cgroup
func (w *CgroupPool) Shutdown() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, c := range w.cgs {
		c.Destroy()
	}
}
