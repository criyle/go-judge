package client

import (
	"time"

	"github.com/criyle/go-judge/pkg/envexec"
	"github.com/criyle/go-sandbox/runner"
)

// ProgressStatus defines progress status
type ProgressStatus int

// Whether progress success / fail
const (
	ProgressSucceeded ProgressStatus = iota + 1
	ProgressFailed
)

// ProgressCompiled compiled progress
type ProgressCompiled struct {
	Status  ProgressStatus
	Message string // compiler output if failed
}

// ProgressProgressed contains progress of current task
type ProgressProgressed struct {
	// defines which test case finished
	SubTaskIndex  int
	TestCaseIndex int

	// test case result
	TestCaseResult
}

// JudgeResult contains final result of current task
type JudgeResult struct {
	SubTasks []SubTaskResult
	Error    string
}

// SubTaskResult contains result for single sub-task
type SubTaskResult struct {
	Score float64
	Cases []TestCaseResult
}

// TestCaseResult contains result for single case
type TestCaseResult struct {
	// status
	Status     ProgressStatus
	ExecStatus envexec.Status

	// message
	Message string
	Error   string

	// score
	ScoreRate float64

	// detail stats
	Time   time.Duration
	Memory runner.Size

	// detail outputs
	Input  []byte
	Answer []byte

	// user stdout / stderr
	UserOutput []byte
	UserError  []byte

	// spj output
	SPJOutput []byte
}
