package envexec

import (
	"context"
	"time"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-sandbox/runner"
)

// Size represent data size in bytes
type Size = runner.Size

// RunnerResult represent process finish result
type RunnerResult = runner.Result

// Cmd defines instruction to run a program in container environment
type Cmd struct {
	// argument, environment
	Args []string
	Env  []string

	// fds for exec: can be nil, file.Opener, PipeCollector
	// nil: undefined, will be closed
	// *os.File: will be fd and passed to runner, file will be close after cmd starts
	// file.Opener: will be opened and passed to runner
	// PipeCollector: a pipe write end will be passed to runner and collected as a copyout file
	Files []interface{}
	TTY   bool // use pty as input / output

	// resource limits
	TimeLimit        time.Duration
	MemoryLimit      Size
	StackLimit       Size
	ExtraMemoryLimit Size
	OutputLimit      Size
	ProcLimit        uint64
	CPURateLimit     float64

	// file contents to copyin before exec
	CopyIn map[string]file.File

	// file names to copyout after exec
	CopyOut    []string
	CopyOutMax Size // file size limit

	// CopyOutDir specifies a dir to dump all /w contnet
	CopyOutDir string

	// Waiter is called after cmd starts and it should return
	// once time limit exceeded.
	// return true to as TLE and false as normal exits (context finished)
	Waiter func(context.Context, Process) bool
}

// PipeCollector can be used in Cmd.Files paramenter
type PipeCollector struct {
	Name      string
	SizeLimit int64
}

// Result defines the running result for single Cmd
type Result struct {
	Status Status

	ExitStatus int

	Error string // error

	Time    time.Duration
	RunTime time.Duration
	Memory  Size // byte

	// Files stores copy out files
	Files map[string]file.File
}
