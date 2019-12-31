package channel

import (
	"github.com/criyle/go-judge/taskqueue"
	"github.com/criyle/go-judge/types"
)

const buffSize = 512

var _ taskqueue.Queue = &Queue{}

// Queue implements taskqueue by buffered go channel
type Queue struct {
	queue chan taskqueue.Task
}

// New creates new Queue with buffed go channel
func New() taskqueue.Queue {
	return &Queue{
		queue: make(chan taskqueue.Task, buffSize),
	}
}

// Send puts task into run queue
func (q *Queue) Send(t types.RunTask) (<-chan types.RunTaskResult, error) {
	c := make(chan types.RunTaskResult, 1)
	q.queue <- Task{
		task:   t,
		result: c,
	}
	return c, nil
}

// ReceiveC returns the underlying channel
func (q *Queue) ReceiveC() <-chan taskqueue.Task {
	return q.queue
}

// Task implements Task interface
type Task struct {
	task   types.RunTask
	result chan<- types.RunTaskResult
}

// Task returns task parameters
func (t Task) Task() *types.RunTask {
	return &t.task
}

// Done returns the run task result
func (t Task) Done(r *types.RunTaskResult) {
	t.result <- *r
}
