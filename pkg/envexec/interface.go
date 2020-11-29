package envexec

import (
	"context"
	"os"
	"time"
)

// ExecveParam is parameters to run process inside environment
type ExecveParam struct {
	// Args holds command line arguments
	Args []string

	// Env specifies the environment of the process
	Env []string

	// Files specifies file descriptors for the child process
	Files []uintptr

	// ExecFile specifies file descriptor for executable file using fexecve
	ExecFile uintptr

	// TTY specifies whether to use TTY
	TTY bool

	// Process Limitations
	Limit Limit
}

// Limit defines the process running resource limits
type Limit struct {
	Time   time.Duration // Time limit
	Memory Size          // Memory limit
	Proc   uint64        // Process count limit
	Stack  Size          // Stack limit
}

// Usage defines the peak process resource usage
type Usage struct {
	Time   time.Duration
	Memory Size
}

// Process reference to the running process group
type Process interface {
	Done() <-chan struct{} // Done returns a channel for wait process to exit
	Result() RunnerResult  // Result is available after done is closed
	Usage() Usage          // Usage retrieves the process usage during the run time
}

// Environment defines the interface to access container execution environment
type Environment interface {
	Execve(context.Context, ExecveParam) (Process, error)
	WorkDir() *os.File // WorkDir returns opened work directory, should not close after
	// Open open file at work dir with given relative path and flags
	Open(path string, flags int, perm os.FileMode) (*os.File, error)
}

// EnvironmentPool implements pool of environments
type EnvironmentPool interface {
	Get() (Environment, error)
	Put(Environment)
}
