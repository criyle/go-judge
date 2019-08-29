package taskqueue

import "github.com/criyle/go-judge/types"

// TaskQueue provides asynchronous message queue to run execution tasks
type TaskQueue interface {
	// Enqueue used to initial a RPC call and receive result to channel
	Enqueue(types.RunTask, chan<- types.RunTaskResult) error
	C() <-chan Task
}

// Task represent a single task to run
type Task interface {
	Task() *types.RunTask
	Finish(*types.RunTaskResult)
}
