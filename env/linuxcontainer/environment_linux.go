package linuxcontainer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/pkg/cgroup"
	"github.com/criyle/go-sandbox/pkg/rlimit"
	"github.com/criyle/go-sandbox/runner"
)

var _ envexec.Environment = &environ{}

// environ defines interface to access container resources
type environ struct {
	container.Environment
	cgPool  CgroupPool
	workDir string
	cpuset  string
	seccomp []syscall.SockFilter
	cpuRate bool
	cgFd    bool
}

// Destroy destroys the environment
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
		cgFd     uintptr
	)

	limit := param.Limit
	if c.cgPool != nil {
		cg, err = c.cgPool.Get()
		if err != nil {
			return nil, fmt.Errorf("execve: failed to get cgroup: %w", err)
		}
		if err := c.setCgroupLimit(cg, limit); err != nil {
			return nil, err
		}
		if c.cgFd {
			f, err := cg.Open()
			if err != nil {
				return nil, fmt.Errorf("execve: failed to get cgroup fd: %w", err)
			}
			defer f.Close()
			cgFd = f.Fd()
		} else {
			syncFunc = cg.AddProc
		}
	}

	rLimits := rlimit.RLimits{
		CPU:         uint64(limit.Time.Truncate(time.Second)/time.Second) + 1,
		FileSize:    limit.Output.Byte(),
		Stack:       limit.Stack.Byte(),
		OpenFile:    limit.OpenFile,
		DisableCore: true,
	}

	if limit.DataSegment || c.cgPool == nil {
		rLimits.Data = limit.Memory.Byte()
	}
	if limit.AddressSpace {
		rLimits.AddressSpace = limit.Memory.Byte()
	}

	// wait for sync or error before turn (avoid file close before pass to child process)
	syncDone := make(chan struct{})

	p := container.ExecveParam{
		Args:     param.Args,
		Env:      param.Env,
		Files:    param.Files,
		CTTY:     param.TTY,
		ExecFile: param.ExecFile,
		RLimits:  rLimits.PrepareRLimit(),
		Seccomp:  c.seccomp,
		SyncFunc: func(pid int) error {
			defer close(syncDone)
			if syncFunc != nil {
				return syncFunc(pid)
			}
			return nil
		},
		SyncAfterExec: syncFunc == nil,
		CgroupFD:      cgFd,
	}
	proc := newProcess(func() runner.Result {
		return c.Environment.Execve(ctx, p)
	}, cg, c.cgPool)

	select {
	case <-proc.done:
	case <-syncDone:
	}

	return proc, nil
}

// Open opens file relative to work directory
func (c *environ) Open(params []envexec.OpenParam) ([]envexec.OpenResult, error) {
	openCmd := make([]container.OpenCmd, 0, len(params))
	for _, p := range params {
		openCmd = append(openCmd, container.OpenCmd{
			Path:     p.Path,
			Flag:     p.Flag,
			Perm:     p.Perm,
			MkdirAll: p.MkdirAll,
		})
	}
	rt, err := c.Environment.Open(openCmd)
	if err != nil {
		return nil, err
	}
	ret := make([]envexec.OpenResult, 0, len(rt))
	for _, r := range rt {
		ret = append(ret, envexec.OpenResult{
			File: r.File,
			Err:  r.Err,
		})
	}
	return ret, nil
}

func (c *environ) Symlink(params []envexec.SymlinkParam) ([]error, error) {
	symlink := make([]container.SymbolicLink, 0, len(params))
	for _, p := range params {
		symlink = append(symlink, container.SymbolicLink{
			LinkPath: p.LinkPath,
			Target:   p.Target,
		})
	}
	return c.Environment.Symlink(symlink)
}

func (c *environ) setCgroupLimit(cg Cgroup, limit envexec.Limit) error {
	cpuSet := limit.CPUSet
	if cpuSet == "" {
		cpuSet = c.cpuset
	}
	if cpuSet != "" {
		if err := cg.SetCpuset(cpuSet); isCgroupSetHasError(err) {
			return fmt.Errorf("execve: cgroup: failed to set cpuset limit: %w", err)
		}
	}
	if c.cpuRate && limit.Rate > 0 {
		if err := cg.SetCPURate(limit.Rate); isCgroupSetHasError(err) {
			return fmt.Errorf("execve: cgroup: failed to set cpu rate limit: %w", err)
		}
	}
	if err := cg.SetMemoryLimit(limit.Memory); isCgroupSetHasError(err) {
		return fmt.Errorf("execve: cgroup: failed to set memory limit: %w", err)
	}
	if err := cg.SetProcLimit(limit.Proc); isCgroupSetHasError(err) {
		return fmt.Errorf("execve: cgroup: failed to set process limit: %w", err)
	}
	return nil
}

func isCgroupSetHasError(err error) bool {
	return err != nil && !errors.Is(err, cgroup.ErrNotInitialized) && !errors.Is(err, os.ErrNotExist)
}
