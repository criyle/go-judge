package types

import "github.com/criyle/go-judge/file"

// ProblemConfig defines a problem
type ProblemConfig struct {
	Type     string    // Problem type
	Subtasks []SubTask // SubTasks

	SPJ        SourceCode // Special Judge
	Interactor SourceCode // Interactor

	ExtraFiles []file.File // Extra Files
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
