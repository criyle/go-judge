package types

import "github.com/criyle/go-judge/file"

// SourceCode defines source code with its language
type SourceCode struct {
	Language   string
	Code       file.File
	ExtraFiles []file.File
}

// JudgeTask contains task received from server
type JudgeTask struct {
	Type        string      // defines problem type
	TestData    []file.File // test data
	Language    string      // code language
	Code        string      // code
	TileLimit   uint64
	MemoryLimit uint64
	Extra       map[string]interface{} // extra parameters
}

// ProgressType defines type of progress message
type ProgressType int

// ProgressType defines type of progress messages
const (
	ProgressStart ProgressType = iota + 1
	ProgressCompiled
	ProgressProgress
	ProgressFinished
	ProgressReported
)

// JudgeProgress contains progress of current task
type JudgeProgress struct {
	Type    ProgressType
	Message string
}

// JudgeResult contains final result of current task
type JudgeResult struct {
	SubTasks []JudgeSubTaskResult
	Error    string
}

// JudgeSubTaskResult contains result for single sub-task
type JudgeSubTaskResult struct {
	Score float64
	Cases []JudgeCaseResult
}

// JudgeCaseResult contains result for single case
type JudgeCaseResult struct {
	Status    string
	Message   string
	ScoreRate float64
	Error     string

	// detail stats
	Time   uint64 // ms
	Memory uint64 // kb

	// detail outputs
	Input      []byte
	Answer     []byte
	UserOutput []byte
	UserError  []byte
	SpjOutput  []byte
}
