package types

import "github.com/criyle/go-judge/file"

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

// JudgeProgress contains progress of current task
type JudgeProgress struct {
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

	// Details
	Time       uint64
	Memory     uint64
	Input      []byte
	Answer     []byte
	UserOutput []byte
	UserError  []byte
	SpjOutput  []byte
}
