package linuxcontainer

import (
	"os"
	"sync"
	"time"
)

var _ CgroupPool = &CachedCgroupPool{}

// CachedCgroupPool keeps freshly created cgroups ready and replaces returned
// cgroups asynchronously.
type CachedCgroupPool struct {
	builder   CgroupBuilder
	cfsPeriod time.Duration

	cache   chan Cgroup
	destroy chan Cgroup
	done    chan struct{}
	wg      sync.WaitGroup
	err     error
}

// NewCachedCgroupPool creates a cached pool with the default cache capacity.
func NewCachedCgroupPool(builder CgroupBuilder, cfsPeriod time.Duration, workerCount int) CgroupPool {
	return NewCachedCgroupPoolWithCapacity(builder, cfsPeriod, workerCount, 4)
}

// NewCachedCgroupPoolWithCapacity creates a cached pool with cacheSize ready
// slots and workerCount background cgroup creators.
func NewCachedCgroupPoolWithCapacity(builder CgroupBuilder, cfsPeriod time.Duration, workerCount, cacheSize int) CgroupPool {
	workerCount = max(workerCount, 1)
	cacheSize = max(cacheSize, 0)
	p := &CachedCgroupPool{
		builder:   builder,
		cfsPeriod: cfsPeriod,
		cache:     make(chan Cgroup, cacheSize),
		destroy:   make(chan Cgroup, max(cacheSize, workerCount)),
		done:      make(chan struct{}),
	}
	p.wg.Add(workerCount)
	for range workerCount {
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
