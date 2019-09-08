package types

import "github.com/criyle/go-judge/file"

// ProblemConfig defines a problem
type ProblemConfig struct {
	Type     string
	Subtasks []SubTask

	SpjLanguage string
	SpjCode     *file.File

	InteractorLanguage string
	InteractorCode     *file.File

	ExtraFiles []file.File
}

// SubTask defines multiple judger tasks
type SubTask struct {
	ScoringType string
	Score       float64
	Cases       []Case
}

// Case defines single judge case
type Case struct {
	Input  file.File
	Answer file.File
}
