package language

// Language defines the way to run program
type Language interface {
	Get(language, t string) ExecParam
}

// ExecParam defines specs to compile / run program
type ExecParam struct {
	SourceFileName    string
	Args              []string
	CompiledFileNames []string

	// limits
	TimeLimit   uint64
	MemoryLimit uint64
	ProcLimit   uint64
	OutputLimit uint64
}
