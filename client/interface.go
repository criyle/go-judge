package client

import "github.com/criyle/go-judge/types"

// Task contains a single task received from web
type Task interface {
	// Param get the judge task
	Param() *types.JudgeTask

	// Progress emits current progress to website
	Progress(*types.JudgeProgress)

	// Finish emits the final result to website
	Finish(*types.JudgeResult)
}

// Client should connect to web service and receive works from web
// it should sent received work through go channel (have background goroutine(s))
type Client interface {
	// C return channel to receive works
	C() <-chan Task
}
