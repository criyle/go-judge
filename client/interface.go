package client

import "github.com/criyle/go-judge/problem"

// Task contains a single task received from web
type Task interface {
	// Param get the judge task
	Param() *JudgeTask

	// Parsed called when problem data have been downloaded and problem
	Parsed(*problem.Config)

	// Compiled called when user code have been compiled (success / fail)
	Compiled(*ProgressCompiled)

	// Progressed called when single test case finished (success / fail + detail message)
	Progressed(*ProgressProgressed)

	// Finished called when all test cases finished / compile failed
	Finished(*JudgeResult)
}

// Client should connect to web service and receive works from web
// it should sent received work through go channel (have background goroutine(s))
type Client interface {
	// C return channel to receive works
	C() <-chan Task
}
