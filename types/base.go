package types

import "github.com/criyle/go-judge/file"

// SourceCode defines source code with its language
type SourceCode struct {
	Language   string
	Code       file.File
	ExtraFiles []file.File
}

// CompiledExec defines compiled executable
type CompiledExec struct {
	Language string
	Exec     []file.File
}
