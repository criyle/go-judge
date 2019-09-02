package taskqueue

import "github.com/criyle/go-judge/types"

// Queue provides asynchronous message queue to run execution tasks
type Queue interface {
	// Enqueue used to initial a RPC call and receive result to channel
	Enqueue(types.RunTask, chan<- types.RunTaskResult) error
	C() <-chan Task
}

// Task represent a single task to run
type Task interface {
	Task() *types.RunTask
	Finish(*types.RunTaskResult)
}
