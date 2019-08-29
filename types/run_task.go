package types

import "github.com/criyle/go-judge/file"

// RunTask is used to send task into RunQueue
// file interface should registed to gob in order
// to encode and decode
type RunTask struct {
	Type string

	// Used for compile task
	Language   string
	Code       string
	ExtraFiles []file.File

	// Used for run tasks
	TimeLimit   uint64
	MemoryLimit uint64
	Executables []file.File
	InputFile   file.File
	AnswerFile  file.File

	// Used for standard run task
	SpjLanguage    string
	SpjExecutables []file.File

	// Used for interaction run task
	InteractorLanguage    string
	InteractorExecutables []file.File

	// Used for answer submission run task
	UserAnswer file.File
}

// RunTaskResult return the result for run task RPC
type RunTaskResult struct {
	Status string
	// error
	Error string
	// details
	Time        uint64
	Memory      uint64
	Input       []byte
	Answer      []byte
	UserOutput  []byte
	UserError   []byte
	SpjOutput   []byte
	ScoringRate float64
}
