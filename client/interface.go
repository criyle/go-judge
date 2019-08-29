package client

import "github.com/criyle/go-judge/types"

// Task contains a single task received from web
type Task interface {
	Param() *types.JudgeTask
	Progress(p types.JudgeProgress)
	Finish(p types.JudgeResult)
}

// Client should connect to web service and receive works from web
// it should sent received work through go channel (have background goroutine(s))
type Client interface {
	// C should return channel to receive works
	C() <-chan types.JudgeTask
}
