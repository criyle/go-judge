package envexec

import (
	"context"
	"os"

	"github.com/criyle/go-sandbox/runner"
)

// runSingle runs Cmd inside the given environment and cgroup
func runSingle(pc context.Context, c *Cmd, fds []*os.File, ptc []pipeCollector, newStoreFile NewStoreFile) (result Result, err error) {
	m := c.Environment
	// copyin
	if fe, err := runSingleCopyIn(m, c.CopyIn); err != nil {
		result.Status = StatusFileError
		result.Error = err.Error()
		result.FileError = fe
		closeFiles(fds...)
		return result, nil
	}
	// symlink
	if fe, err := symlink(m, c.SymLinks); err != nil {
		result.Status = StatusFileError
		result.Error = err.Error()
		result.FileError = []FileError{*fe}
		closeFiles(fds...)
		return result, nil
	}

	// run cmd and wait for result
	rt := runSingleWait(pc, m, c, fds)

	// collect result
	files, fe, err := copyOutAndCollect(m, c, ptc, newStoreFile)
	result = Result{
		Status:     convertStatus(rt.Status),
		ExitStatus: rt.ExitStatus,
		Error:      rt.Error,
		Time:       rt.Time,
		RunTime:    rt.RunningTime,
		Memory:     rt.Memory,
		Files:      files,
		FileError:  fe,
	}
	// collect error (only if the process exits normally)
	if rt.Status == runner.StatusNormal && err != nil && result.Error == "" {
		switch err := err.(type) {
		case runner.Status:
			result.Status = convertStatus(err)
		default:
			result.Status = StatusFileError
		}
		result.Error = err.Error()
	}
	if result.Time > c.TimeLimit {
		result.Status = StatusTimeLimitExceeded
	}
	if result.Memory > c.MemoryLimit {
		result.Status = StatusMemoryLimitExceeded
	}
	return result, nil
}

func runSingleCopyIn(m Environment, copyInFiles map[string]File) ([]FileError, error) {
	if len(copyInFiles) == 0 {
		return nil, nil
	}
	return copyIn(m, copyInFiles)
}

func runSingleWait(pc context.Context, m Environment, c *Cmd, fds []*os.File) RunnerResult {
	// start the cmd (they will be canceled in other goroutines)
	ctx, cancel := context.WithCancel(pc)
	defer cancel()

	process, err := runSingleExecve(ctx, m, c, fds)
	if err != nil {
		return runner.Result{
			Status: runner.StatusRunnerError,
			Error:  err.Error(),
		}
	}

	// starts waiter to periodically check cpu usage
	c.Waiter(ctx, process)
	// cancel the process as waiter exits
	cancel()

	return process.Result()
}

func runSingleExecve(ctx context.Context, m Environment, c *Cmd, fds []*os.File) (Process, error) {
	defer closeFiles(fds...)

	extraMemoryLimit := c.ExtraMemoryLimit
	if extraMemoryLimit == 0 {
		extraMemoryLimit = defaultExtraMemoryLimit
	}

	memoryLimit := c.MemoryLimit + extraMemoryLimit

	var stackLimit Size
	if c.StackLimit > 0 {
		stackLimit = c.StackLimit
	}
	if stackLimit > memoryLimit {
		stackLimit = memoryLimit
	}

	// set running parameters
	execParam := ExecveParam{
		Args:  c.Args,
		Env:   c.Env,
		Files: getFdArray(fds),
		TTY:   c.TTY,
		Limit: Limit{
			Time:         c.TimeLimit,
			Memory:       memoryLimit,
			Proc:         c.ProcLimit,
			Stack:        stackLimit,
			Output:       c.OutputLimit,
			Rate:         c.CPURateLimit,
			OpenFile:     c.OpenFileLimit,
			CPUSet:       c.CPUSetLimit,
			StrictMemory: c.StrictMemoryLimit,
		},
	}
	return m.Execve(ctx, execParam)
}
