package runner

import (
	"sync"

	"github.com/criyle/go-judge/language"
	"github.com/criyle/go-judge/taskqueue"
)

// Runner is the task runner
type Runner struct {
	Queue    taskqueue.Queue
	Language language.Language
	Root     string
	pool     *pool
	once     sync.Once
}

func (r *Runner) init() {
	r.pool = newPool(r.Root)
}

// Loop status a runner in a forever loop, waiting for task and execute
// call it in new goroutine
func (r *Runner) Loop(done <-chan struct{}) error {
	r.once.Do(r.init)
	c := r.Queue.C()
loop:
	for {
		select {
		case <-done:
			break loop
		case task := <-c:
			select {
			case <-done:
				break loop
			default:
				task.Finish(r.run(done, task.Task()))
			}
		}
	}
	return nil
}
