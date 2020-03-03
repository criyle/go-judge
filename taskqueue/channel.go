package taskqueue

import (
	"github.com/criyle/go-judge/runner"
)

var _ runner.Queue = &ChannelQueue{}

// ChannelQueue implements taskqueue by buffered go channel
type ChannelQueue struct {
	queue chan runner.Task
}

// NewChannelQueue creates new Queue with buffed go channel
func NewChannelQueue(size int) runner.Queue {
	return &ChannelQueue{
		queue: make(chan runner.Task, size),
	}
}

// Send puts task into run queue
func (q *ChannelQueue) Send(t runner.RunTask) (<-chan runner.RunTaskResult, error) {
	c := make(chan runner.RunTaskResult, 1)
	q.queue <- Task{
		task:   t,
		result: c,
	}
	return c, nil
}

// ReceiveC returns the underlying channel
func (q *ChannelQueue) ReceiveC() <-chan runner.Task {
	return q.queue
}

// Task implements Task interface
type Task struct {
	task   runner.RunTask
	result chan<- runner.RunTaskResult
}

// Task returns task parameters
func (t Task) Task() *runner.RunTask {
	return &t.task
}

// Done returns the run task result
func (t Task) Done(r *runner.RunTaskResult) {
	t.result <- *r
}
