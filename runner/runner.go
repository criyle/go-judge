package runner

import (
	"sync"

	"github.com/criyle/go-judge/language"
	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/pkg/cgroup"
)

// EnvironmentBuilder defines the abstract builder for container environment
type EnvironmentBuilder interface {
	Build() (container.Environment, error)
}

// CgroupBuilder builds cgroup for runner
type CgroupBuilder interface {
	Build() (cg *cgroup.Cgroup, err error)
}

// Runner is the task runner
type Runner struct {
	// Queue is the message queue to receive run tasks
	Queue Receiver

	// Builder builds the sandbox runner
	Builder EnvironmentBuilder

	// Builder for cgroups
	CgroupBuilder CgroupBuilder

	Language language.Language

	// pool of sandbox to use
	pool *pool

	cgPool *fCgroupPool

	// ensure init / shutdown only once
	onceInit, onceShutdown sync.Once
}

func (r *Runner) init() {
	r.pool = newPool(r.Builder)
	r.cgPool = newFakeCgroupPool(r.CgroupBuilder)
}

// Loop status a runner in a forever loop, waiting for task and execute
// call it in new goroutine
func (r *Runner) Loop(done <-chan struct{}) {
	r.onceInit.Do(r.init)
	c := r.Queue.ReceiveC()
loop:
	for {
		select {
		case <-done:
			break loop

		case task := <-c:
			task.Done(r.run(done, task.Task()))
		}

		// check if cancel is signaled
		select {
		case <-done:
			break loop

		default:
		}
	}
	r.onceShutdown.Do(func() {
		r.cgPool.Shutdown()
		r.pool.Shutdown()
	})
}
