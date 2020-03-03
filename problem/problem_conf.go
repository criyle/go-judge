package problem

import "github.com/criyle/go-judge/file"

// Config defines a problem judgement configuration
type Config struct {
	Type     string    // Problem type
	Subtasks []SubTask // SubTasks

	SPJ        file.SourceCode // Special Judge
	Interactor file.SourceCode // Interactor

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
