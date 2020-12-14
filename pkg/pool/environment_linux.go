package pool

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/criyle/go-judge/pkg/envexec"
	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/pkg/rlimit"
)

var _ envexec.Environment = &environ{}

// environ defines interface to access container resources
type environ struct {
	container.Environment
	cgPool  CgroupPool
	wd      *os.File // container work dir
	cpuset  string
	seccomp []syscall.SockFilter
	cpuRate bool
}

// Destory destories the environment
func (c *environ) Destory() error {
	return c.Environment.Destroy()
}

func (c *environ) Reset() error {
	return c.Environment.Reset()
}

// Execve execute process inside the environment
func (c *environ) Execve(ctx context.Context, param envexec.ExecveParam) (envexec.Process, error) {
	var (
		cg       Cgroup
		syncFunc func(int) error
		err      error
	)

	if c.cgPool != nil {
		cg, err = c.cgPool.Get()
		if err != nil {
			return nil, fmt.Errorf("execve: failed to get cgroup %v", err)
		}
		if c.cpuset != "" {
			cg.SetCpuset(c.cpuset)
		}
		if c.cpuRate && param.Limit.Rate > 0 {
			cg.SetCPURate(param.Limit.Rate)
		}
		cg.SetMemoryLimit(param.Limit.Memory)
		cg.SetProcLimit(param.Limit.Proc)
		syncFunc = cg.AddProc
	}

	rLimits := rlimit.RLimits{
		CPU:      uint64(param.Limit.Time.Truncate(time.Second)/time.Second) + 1,
		Data:     param.Limit.Memory.Byte(),
		FileSize: param.Limit.Output.Byte(),
		Stack:    param.Limit.Stack.Byte(),
	}

	p := container.ExecveParam{
		Args:     param.Args,
		Env:      param.Env,
		Files:    param.Files,
		CTTY:     param.TTY,
		ExecFile: param.ExecFile,
		RLimits:  rLimits.PrepareRLimit(),
		Seccomp:  c.seccomp,
		SyncFunc: syncFunc,
	}
	rt := c.Environment.Execve(ctx, p)
	return newProcess(rt, cg, c.cgPool), nil
}

// WorkDir returns opened work directory, should not close after
func (c *environ) WorkDir() *os.File {
	c.wd.Seek(0, 0)
	return c.wd
}

// Open opens file relative to work directory
func (c *environ) Open(path string, flags int, perm os.FileMode) (*os.File, error) {
	fd, err := syscall.Openat(int(c.wd.Fd()), path, flags|syscall.O_CLOEXEC, uint32(perm))
	if err != nil {
		return nil, fmt.Errorf("openAtWorkDir: %v", err)
	}
	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		return nil, fmt.Errorf("openAtWorkDir: failed to NewFile")
	}
	return f, nil
}
