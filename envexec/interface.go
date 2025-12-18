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
	Time         time.Duration // Time limit
	Memory       Size          // Memory limit
	Proc         uint64        // Process count limit
	Stack        Size          // Stack limit
	Output       Size          // Output limit
	Rate         uint64        // CPU Rate limit
	OpenFile     uint64        // Number of open files
	CPUSet       string        // CPU set limit
	DataSegment  bool          // Use stricter memory limit (e.g. rlimit)
	AddressSpace bool          // rlimit address space
}

// Usage defines the peak process resource usage
type Usage struct {
	Time   time.Duration
	Memory Size
}

// Process reference to the running process group
type Process interface {
	Done() <-chan struct{} // Done returns a channel for wait process to exit
	Result() RunnerResult  // Result wait until done and returns RunnerResult
	Usage() Usage          // Usage retrieves the process usage during the run time
}

// OpenParam represent a open call in the environment
type OpenParam struct {
	Path     string
	Flag     int
	Perm     os.FileMode
	MkdirAll bool
}

// OpenResult represent a result from a open call, it should
// be either a opened file or an error
type OpenResult struct {
	File *os.File
	Err  error
}

// SymlinkParam represent parameters for symlink call
type SymlinkParam struct {
	LinkPath string
	Target   string
}

// Environment defines the interface to access container execution environment
type Environment interface {
	Execve(context.Context, ExecveParam) (Process, error)
	// Open open file at work dir with given relative path and flags
	Open([]OpenParam) ([]OpenResult, error)
	// Make symbolic link for a file / directory
	Symlink([]SymlinkParam) []error
}

// NewStoreFile creates a new file in storage
type NewStoreFile func() (*os.File, error)
