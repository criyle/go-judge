package types

import "github.com/criyle/go-judge/file"

// RunTask is used to send task into RunQueue,
// if taskqueue is a remote queue, taskqueue need to store / retrive files
type RunTask struct {
	Type     string // compile / standard / interactive / answer_submit
	Language string // task programming language

	// Used for compile task
	Code       string
	ExtraFiles []file.File

	// Used for run tasks
	TimeLimit   uint64 // ms
	MemoryLimit uint64 // kb
	ExecFiles   []file.File
	InputFile   file.File
	AnswerFile  file.File

	// Used for standard run task
	SPJ SourceCode

	// Used for interaction run task
	Interactor SourceCode

	// Used for answer submission run task
	UserAnswer file.File
}

// RunTaskResult return the result for run task RPC
type RunTaskResult struct {
	// status
	Status string

	// score
	ScoringRate float64

	// error
	Error string

	// compile result
	ExecFiles []file.File

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
