package linuxcontainer

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/criyle/go-judge/envexec"
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

// Destroy destories the environment
func (c *environ) Destroy() error {
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

	limit := param.Limit
	if c.cgPool != nil {
		cg, err = c.cgPool.Get()
		if err != nil {
			return nil, fmt.Errorf("execve: failed to get cgroup %v", err)
		}
		if c.cpuset != "" {
			cg.SetCpuset(c.cpuset)
		}
		if c.cpuRate && limit.Rate > 0 {
			cg.SetCPURate(limit.Rate)
		}
		cg.SetMemoryLimit(limit.Memory)
		cg.SetProcLimit(limit.Proc)
		syncFunc = cg.AddProc
	}

	rLimits := rlimit.RLimits{
		CPU:         uint64(limit.Time.Truncate(time.Second)/time.Second) + 1,
		FileSize:    limit.Output.Byte(),
		Stack:       limit.Stack.Byte(),
		DisableCore: true,
	}

	if limit.StrictMemory || c.cgPool == nil {
		rLimits.Data = limit.Memory.Byte()
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
