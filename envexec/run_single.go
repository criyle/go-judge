package envexec

import (
	"context"
	"os"

	"github.com/criyle/go-sandbox/runner"
)

// runSingle runs Cmd inside the given environment and cgroup
func runSingle(pc context.Context, c *Cmd, fds []*os.File, ptc []pipeCollector) (result Result, err error) {
	m := c.Environment
	// copyin
	if err := runSingleCopyIn(m, c.CopyIn); err != nil {
		result.Status = StatusFileError
		result.Error = err.Error()
		closeFiles(fds...)
		return result, nil
	}

	// run cmd and wait for result
	rt := runSingleWait(pc, m, c, fds)

	// collect result
	files, err := copyOutAndCollect(m, c, ptc)
	result = Result{
		Status:     convertStatus(rt.Status),
		ExitStatus: rt.ExitStatus,
		Error:      rt.Error,
		Time:       rt.Time,
		RunTime:    rt.RunningTime,
		Memory:     rt.Memory,
		Files:      files,
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

func runSingleCopyIn(m Environment, copyInFiles map[string]File) error {
	if len(copyInFiles) == 0 {
		return nil
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
	go func() {
		defer cancel()
		c.Waiter(ctx, process)
	}()

	// ensure waiter exit
	<-ctx.Done()
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
			Rate:         c.CPURateLimit,
			StrictMemory: c.StrictMemoryLimit,
		},
	}
	return m.Execve(ctx, execParam)
}
