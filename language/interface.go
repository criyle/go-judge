package language

import (
	"time"

	"github.com/criyle/go-sandbox/runner"
)

// Type defines compile / exec
type Type int

// Defines the exec type
const (
	TypeCompile Type = iota + 1
	TypeExec
)

// Language defines the way to run program
type Language interface {
	Get(string, Type) ExecParam // Get execparam for specific language and type (compile / run)
}

// ExecParam defines specs to compile / run program
type ExecParam struct {
	Args []string
	Env  []string

	// Compile
	SourceFileName    string   // put code when compile
	CompiledFileNames []string // exec files

	// limits
	TimeLimit   time.Duration
	MemoryLimit runner.Size
	ProcLimit   uint64
	OutputLimit runner.Size
}
