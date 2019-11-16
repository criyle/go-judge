package runner

import (
	"sync"

	"github.com/criyle/go-judge/language"
	"github.com/criyle/go-judge/taskqueue"
	"github.com/criyle/go-sandbox/daemon"
	"github.com/criyle/go-sandbox/pkg/cgroup"
)

// DaemonBuilder defines the abstract builder for daemon
type DaemonBuilder interface {
	Build() (*daemon.Master, error)
}

// CgroupBuilder builds cgroup for runner
type CgroupBuilder interface {
	Build() (cg *cgroup.CGroup, err error)
}

// Runner is the task runner
type Runner struct {
	// Queue is the message queue to receive run tasks
	Queue taskqueue.Receiver

	// Builder builds the sandbox runner
	Builder DaemonBuilder

	// Builder for cgroups
	CgroupBuilder CgroupBuilder

	Language language.Language

	// pool of sandbox to use
	pool *pool
	once sync.Once
}

func (r *Runner) init() {
	r.pool = newPool(r.Builder)
}

// Loop status a runner in a forever loop, waiting for task and execute
// call it in new goroutine
func (r *Runner) Loop(done <-chan struct{}) error {
	r.once.Do(r.init)
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
	return nil
}
