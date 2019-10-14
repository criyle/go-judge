package runner

import (
	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-sandbox/daemon"
)

// Pool implements pool of daemons
type Pool interface {
	Get() (*daemon.Master, error)
	Put(*daemon.Master)
}

// PipeCollector can be used in Cmd.Files paramenter
type PipeCollector struct {
	Name      string
	SizeLimit uint64
}

// Pipe defines the pipe between parallel Cmd
type Pipe struct {
	ReadIndex  int
	ReadFd     int
	WriteIndex int
	WriteFd    int
}

// Cmd defines instruction to run a program in daemon
type Cmd struct {
	// argument, environment
	Args []string
	Env  []string

	// fds for exec: can be nil, file.Opener, PipeCollector
	// nil: undefined, will be closed
	// file.Opener: will be opened and passed to runner
	// PipeCollector: a pipe write end will be passed to runner and collected as a copyout file
	Files []interface{}

	// cgroup limits
	MemoryLimit uint64
	PidLimit    uint64

	// file contents to copyin before exec
	CopyIn map[string]file.File

	// file names to copyout after exec
	CopyOut []string
}

// Runner defines the running instruction to run multiple
// Exec in parallel restricted within cgroup
type Runner struct {
	CgroupPrefix string
	Pool         Pool

	Cmds  []Cmd
	Pipes []Pipe
}
