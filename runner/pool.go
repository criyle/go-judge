package runner

import (
	"sync/atomic"

	"github.com/criyle/go-sandbox/deamon"
)

type pool struct {
	queue chan *deamon.Master
	count int32
	root  string
}

const maxPoolSize = 64

func newPool(root string) *pool {
	return &pool{
		queue: make(chan *deamon.Master, maxPoolSize),
		root:  root,
	}
}

func (p *pool) Get() (*deamon.Master, error) {
	select {
	case m := <-p.queue:
		return m, nil
	default:
	}
	atomic.AddInt32(&p.count, 1)
	return deamon.New(p.root)
}

func (p *pool) Put(master *deamon.Master) {
	p.queue <- master
}

func (p *pool) Destroy(master *deamon.Master) {
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
