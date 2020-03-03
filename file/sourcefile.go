package file

// SourceCode defines source code with its language
type SourceCode struct {
	Language   string
	Code       File
	ExtraFiles []File
}

// CompiledExec defines compiled executable
type CompiledExec struct {
	Language string
	Exec     []File
}
