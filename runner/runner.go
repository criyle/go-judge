package runner

import (
	"sync"

	"github.com/criyle/go-judge/language"
	"github.com/criyle/go-judge/pkg/pool"
)

// Runner is the task runner
type Runner struct {
	// Queue is the message queue to receive run tasks
	Queue Receiver

	// Builder builds the sandbox runner
	Builder pool.EnvironmentBuilder

	// Builder for cgroups
	CgroupBuilder pool.CgroupBuilder

	Language language.Language

	// pool of sandbox to use
	pool *pool.EnvPool

	cgPool *pool.FakeCgroupPool

	// ensure init / shutdown only once
	onceInit, onceShutdown sync.Once
}

func (r *Runner) init() {
	r.pool = pool.NewEnvPool(r.Builder)
	r.cgPool = pool.NewFakeCgroupPool(r.CgroupBuilder)
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
