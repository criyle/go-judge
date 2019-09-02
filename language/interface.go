package language

// Language defines the way to run program
type Language interface {
	Get(string, string) ExecParam // Get execparam for specific language and type (compile / run)
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
