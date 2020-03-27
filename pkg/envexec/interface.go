package envexec

import (
	"os"
	"time"

	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/runner"
)

// Environment defines the interface to access container execution environment
type Environment interface {
	container.Environment

	// WorkDir returns opened work directory, should not close after
	WorkDir() *os.File

	// OpenAtWorkDir open file at work dir with given relative path and flags
	OpenAtWorkDir(path string, flags int, perm os.FileMode) (*os.File, error)
}

// EnvironmentPool implements pool of environments
type EnvironmentPool interface {
	Get() (Environment, error)
	Put(Environment)
}

// Cgroup defines interface to limit and monitor resources consumption of a process
type Cgroup interface {
	SetMemoryLimit(runner.Size) error
	SetProcLimit(uint64) error

	CPUUsage() (time.Duration, error)
	MemoryUsage() (runner.Size, error)

	AddProc(int) error
	Reset() error
	Destroy() error
}

// CPUUsager access process cpu usage (from cgroup)
type CPUUsager interface {
	CPUUsage() (time.Duration, error)
}

// CgroupPool implements pool of Cgroup
type CgroupPool interface {
	Get() (Cgroup, error)
	Put(Cgroup)
}
