package runner

import (
	"github.com/criyle/go-judge/language"
	"github.com/criyle/go-judge/taskqueue"
)

// Runner is the task runner
type Runner struct {
	Queue    taskqueue.TaskQueue
	Language language.Language
	Root     string
}

// Loop status a runner in a forever loop, waiting for task and execute
func (r *Runner) Loop(done <-chan struct{}) error {
	c := r.Queue.C()
	i, err := r.newInstance()
	if err != nil {
		return err
	}
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
				task.Finish(i.run(done, task.Task()))
			}
		}
	}
	return nil
}
