package runner

import (
	"time"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-judge/types"
	"github.com/criyle/go-sandbox/daemon"
	"github.com/criyle/go-sandbox/pkg/cgroup"
)

// Pool implements pool of daemons
type Pool interface {
	Get() (*daemon.Master, error)
	Put(*daemon.Master)
}

// PipeCollector can be used in Cmd.Files paramenter
type PipeCollector struct {
	Name      string
	SizeLimit int64
}

// PipeIndex defines the index of cmd and the fd of the that cmd
type PipeIndex struct {
	Index int
	Fd    int
}

// Pipe defines the pipe between parallel Cmd
type Pipe struct {
	In, Out PipeIndex
}

// CPUAcctor access process cpu usage in ns
type CPUAcctor interface {
	CpuacctUsage() (uint64, error)
}

// Cmd defines instruction to run a program in daemon
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

	// cgroup limits
	MemoryLimit uint64 // in bytes
	PidLimit    uint64

	// file contents to copyin before exec
	CopyIn map[string]file.File

	// file names to copyout after exec
	CopyOut []string

	// Waiter is called after cmd starts and it should return
	// once time limit exceeded.
	// return true to as TLE and false as normal exits
	Waiter func(chan struct{}, CPUAcctor) bool
}

// CgroupBuilder builds cgroup for runner
type CgroupBuilder interface {
	Build() (cg *cgroup.CGroup, err error)
}

// Runner defines the running instruction to run multiple
// Exec in parallel restricted within cgroup
type Runner struct {
	// CGBuilder defines cgroup builder used for Cmd
	// must have cpu, memory and pids sub-cgroup
	CGBuilder CgroupBuilder

	// MasterPool defines pool used for runner environment
	MasterPool Pool

	// Cmds defines Cmd running in parallel
	Cmds []*Cmd

	// Pipes defines the potential mapping between Cmds.
	// ensure nil is used as placeholder in Cmd
	Pipes []*Pipe
}

// Result defines the running result for single Cmd
type Result struct {
	Status types.Status

	Error string // error

	Time   time.Duration
	Memory uint64 // byte

	// Files stores copy out files
	Files map[string]file.File
}
