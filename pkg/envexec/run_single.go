package envexec

import (
	"context"
	"os"

	"github.com/criyle/go-sandbox/runner"
)

// runSingle runs Cmd inside the given environment and cgroup
func runSingle(pc context.Context, m Environment, c *Cmd, fds []*os.File, ptc []pipeCollector) (result Result, err error) {
	fdToClose := fds
	defer func() { closeFiles(fdToClose) }()

	// copyin
	if len(c.CopyIn) > 0 {
		if err := copyIn(m, c.CopyIn); err != nil {
			result.Status = StatusInternalError
			result.Error = err.Error()
			return result, nil
		}
	}

	extraMemoryLimit := c.ExtraMemoryLimit
	if extraMemoryLimit == 0 {
		extraMemoryLimit = defaultExtraMemoryLimit
	}

	memoryLimit := c.MemoryLimit + extraMemoryLimit

	var stackLimit Size
	if c.StackLimit > 0 {
		stackLimit = c.StackLimit + extraMemoryLimit
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
			Time:   c.TimeLimit,
			Memory: memoryLimit,
			Proc:   c.ProcLimit,
			Stack:  stackLimit,
		},
	}

	// start the cmd (they will be canceled in other goroutines)
	ctx, cancel := context.WithCancel(pc)
	waiterCtx, waiterCancel := context.WithCancel(ctx)

	process, err := m.Execve(ctx, execParam)

	// close files
	closeFiles(fds)
	fdToClose = nil

	// starts waiter to periodically check cpu usage
	go func() {
		c.Waiter(waiterCtx, process)
		cancel()
	}()

	var rt runner.Result
	if err == nil {
		<-process.Done()
		rt = process.Result()
	} else {
		rt = runner.Result{
			Status: runner.StatusRunnerError,
			Error:  err.Error(),
		}
	}

	waiterCancel()

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
	// make sure waiter exit
	<-ctx.Done()
	return result, nil
}
