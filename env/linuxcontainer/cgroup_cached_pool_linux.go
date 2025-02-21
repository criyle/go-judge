package linuxcontainer

import (
	"os"
	"sync"
	"time"
)

var _ CgroupPool = &CachedCgroupPool{}

// CachedCgroupPool implements cgroup pool but not actually do pool
type CachedCgroupPool struct {
	builder   CgroupBuilder
	cfsPeriod time.Duration

	cache   chan Cgroup
	destroy chan Cgroup
	done    chan struct{}
	wg      sync.WaitGroup
	err     error
}

// NewFakeCgroupPool creates FakeCgroupPool
func NewCachedCgroupPool(builder CgroupBuilder, cfsPeriod time.Duration, workerCount int) CgroupPool {
	p := &CachedCgroupPool{
		builder:   builder,
		cfsPeriod: cfsPeriod,
		cache:     make(chan Cgroup, 4),
		destroy:   make(chan Cgroup, 4),
		done:      make(chan struct{}),
	}
	p.wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go p.loop()
	}
	return p
}

func (p *CachedCgroupPool) loop() {
	defer p.wg.Done()
	var cache Cgroup
	for {
		if cache == nil {
			cg, err := p.builder.Random("")
			if err != nil {
				p.err = err
				close(p.done)
				return
			}
			cache = &wCgroup{cg: cg, cfsPeriod: p.cfsPeriod}
		}

		select {
		case <-p.done:
			return

		case c := <-p.destroy:
			c.Destroy()

		case p.cache <- cache:
			cache = nil
		}
	}
}

// Get gets new cgroup
func (p *CachedCgroupPool) Get() (Cgroup, error) {
	select {
	case <-p.done:
		if p.err == nil {
			return nil, os.ErrClosed
		}
		return nil, p.err
	case c := <-p.cache:
		return c, nil
	}
}

// Put destroy the cgroup
func (p *CachedCgroupPool) Put(c Cgroup) {
	p.destroy <- c
}

// Shutdown noop
func (p *CachedCgroupPool) Shutdown() {
	close(p.done)
	p.wg.Wait()

	// drain all cgroups
	close(p.cache)
	for c := range p.cache {
		c.Destroy()
	}
	close(p.destroy)
	for c := range p.destroy {
		c.Destroy()
	}
}
