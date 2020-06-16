package worker

import "github.com/criyle/go-judge/pkg/envexec"

// Cmd defines command and limits to start a program using in envexec
type Cmd struct {
	Args  []string
	Env   []string
	Files []CmdFile
	TTY   bool

	CPULimit     uint64
	RealCPULimit uint64
	MemoryLimit  uint64
	StackLimit   uint64
	ProcLimit    uint64

	CopyIn map[string]CmdFile

	CopyOut       []string
	CopyOutCached []string
	CopyOutMax    uint64
	CopyOutDir    string
}

// PipeIndex defines indexing for a pipe fd
type PipeIndex struct {
	Index int
	Fd    int
}

// PipeMap defines in / out pipe for multiple program
type PipeMap struct {
	In  PipeIndex
	Out PipeIndex
}

// Request defines single worker request
type Request struct {
	RequestID   string
	Cmd         []Cmd
	PipeMapping []PipeMap
}

// Result defines single command response
type Result struct {
	Status     envexec.Status
	ExitStatus int
	Error      string
	Time       uint64
	RunTime    uint64
	Memory     uint64
	Files      map[string][]byte
	FileIDs    map[string]string
}

// Response defines worker response for single request
type Response struct {
	RequestID string
	Results   []Result
	Error     error
}
