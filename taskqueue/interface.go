package taskqueue

import "github.com/criyle/go-judge/types"

// Sender interface is used to send run task into message queue
type Sender interface {
	// Send used to initial a RPC call and receive result to channel (should have enough space)
	Send(types.RunTask, chan<- types.RunTaskResult) error
}

// Receiver interface is used to receive run task from message queue
type Receiver interface {
	// ReceiveC get the channel to receive tasks
	ReceiveC() <-chan Task
}

// Queue provides asynchronous message queue for run tasks
type Queue interface {
	Sender
	Receiver
}

// Task represent a single task to run
type Task interface {
	// Task gets the run task parameter
	Task() *types.RunTask

	// Done returns the run task result for the run task (should be called only once at end)
	Done(*types.RunTaskResult)
}
