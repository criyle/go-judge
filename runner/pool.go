package runner

import (
	"sync/atomic"

	"github.com/criyle/go-sandbox/daemon"
)

type pool struct {
	builder DaemonBuilder
	queue   chan *daemon.Master
	count   int32
}

const maxPoolSize = 64

func newPool(builder DaemonBuilder) *pool {
	return &pool{
		queue:   make(chan *daemon.Master, maxPoolSize),
		builder: builder,
	}
}

func (p *pool) Get() (*daemon.Master, error) {
	select {
	case m := <-p.queue:
		return m, nil
	default:
	}
	atomic.AddInt32(&p.count, 1)
	return p.builder.Build()
}

func (p *pool) Put(master *daemon.Master) {
	master.Reset()
	p.queue <- master
}

func (p *pool) Destroy(master *daemon.Master) {
	master.Destroy()
	atomic.AddInt32(&p.count, -1)
}

func (p *pool) Release() {
	for {
		select {
		case m := <-p.queue:
			p.Destroy(m)

		default:
			return
		}
	}
}

func (p *pool) Shutdown() {
	for atomic.LoadInt32(&p.count) > 0 {
		m := <-p.queue
		p.Destroy(m)
	}
}
