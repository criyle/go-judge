package runner

import (
	"sync/atomic"

	"github.com/criyle/go-sandbox/daemon"
)

type pool struct {
	queue chan *daemon.Master
	count int32
	root  string
}

const maxPoolSize = 64

func newPool(root string) *pool {
	return &pool{
		queue: make(chan *daemon.Master, maxPoolSize),
		root:  root,
	}
}

func (p *pool) Get() (*daemon.Master, error) {
	select {
	case m := <-p.queue:
		return m, nil
	default:
	}
	atomic.AddInt32(&p.count, 1)
	return daemon.New(p.root)
}

func (p *pool) Put(master *daemon.Master) {
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

func (p *pool) DestroyAllAndWait() {
	for p.count > 0 {
		m := <-p.queue
		p.Destroy(m)
	}
}
