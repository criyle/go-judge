package envexec

import (
	"time"

	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/runner"
)

// EnvironmentPool implements pool of environments
type EnvironmentPool interface {
	Get() (container.Environment, error)
	Put(container.Environment)
}

// Cgroup defines interface to limit and monitor resources consumption of a process
type Cgroup interface {
	SetMemoryLimit(runner.Size) error
	SetProcLimit(uint64) error

	CPUUsage() (time.Duration, error)
	MemoryUsage() (runner.Size, error)

	AddProc(int) error
	Reset() error
	Destory() error
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
