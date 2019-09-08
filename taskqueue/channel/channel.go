package channel

import (
	"github.com/criyle/go-judge/taskqueue"
	"github.com/criyle/go-judge/types"
)

const buffSize = 512

// Queue implements taskqueue by go channel
type Queue struct {
	queue chan taskqueue.Task
}

// New craetes new Queue with buffed go channel
func New() *Queue {
	return &Queue{
		queue: make(chan taskqueue.Task, buffSize),
	}
}

// Enqueue puts task into run queue
func (q *Queue) Enqueue(t types.RunTask, r chan<- types.RunTaskResult) error {
	q.queue <- Task{
		task:   t,
		result: r,
	}
	return nil
}

// C returns the underlying channel
func (q *Queue) C() <-chan taskqueue.Task {
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

// Finish returns the run task result
func (t Task) Finish(r *types.RunTaskResult) {
	t.result <- *r
}
