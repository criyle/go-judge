package envexec

import (
	"context"
	"os"

	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/runner"
)

// runSingle runs Cmd inside the given environment and cgroup
func runSingle(m Environment, cg Cgroup, c *Cmd, fds []*os.File, ptc []pipeCollector) (result Result, err error) {
	fdToClose := fds
	defer func() { closeFiles(fdToClose) }()

	// setup cgroup limits
	if err := cg.SetMemoryLimit(runner.Size(uint64(c.MemoryLimit) + memoryLimitExtra)); err != nil {
		return result, err
	}
	if err := cg.SetProcLimit(c.ProcLimit); err != nil {
		return result, err
	}

	// copyin
	if len(c.CopyIn) > 0 {
		if err := copyIn(m, c.CopyIn); err != nil {
			return result, err
		}
	}

	// set running parameters
	execParam := container.ExecveParam{
		Args:     c.Args,
		Env:      c.Env,
		Files:    getFdArray(fds),
		SyncFunc: cg.AddProc,
	}

	// start the cmd (they will be canceled in other goroutines)
	ctx, cancel := context.WithCancel(context.TODO())
	waiterCtx, waiterCancel := context.WithCancel(ctx)

	rc := m.Execve(ctx, execParam)

	// close files
	closeFiles(fds)
	fdToClose = nil

	// starts waiter to periodically check cpu usage
	go func() {
		c.Waiter(waiterCtx, cg)
		cancel()
	}()

	rt := <-rc
	waiterCancel()

	// collect result
	files, err := copyOutAndCollect(m, c, ptc)
	result = Result{
		Status:     convertStatus(rt.Status),
		ExitStatus: rt.ExitStatus,
		Error:      rt.Error,
		Time:       rt.Time,
		Memory:     rt.Memory,
		Files:      files,
	}
	// collect error
	if err != nil && result.Error == "" {
		switch err := err.(type) {
		case runner.Status:
			result.Status = convertStatus(err)
		default:
			result.Status = StatusFileError
		}
		result.Error = err.Error()
	}
	// time
	cpuUsage, err := cg.CPUUsage()
	if err != nil {
		result.Status = StatusInternalError
		result.Error = err.Error()
	} else {
		result.Time = cpuUsage
	}
	// memory
	memoryUsage, err := cg.MemoryUsage()
	if err != nil {
		result.Status = StatusInternalError
		result.Error = err.Error()
	} else {
		result.Memory = memoryUsage
	}
	if result.Memory > c.MemoryLimit {
		result.Status = StatusMemoryLimitExceeded
	}
	// make sure waiter exit
	<-ctx.Done()
	return result, nil
}
