package runner

import (
	"time"

	"github.com/criyle/go-sandbox/pkg/cgroup"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-judge/file/memfile"
	"github.com/criyle/go-judge/language"
	"github.com/criyle/go-judge/pkg/runner"
	"github.com/criyle/go-judge/types"
)

const maxOutput = 4 << 20 // 4M

func (r *Runner) run(done <-chan struct{}, task *types.RunTask) *types.RunTaskResult {
	t := language.TypeExec
	if task.Type == "compile" {
		t = language.TypeCompile
	}
	param := r.Language.Get(task.Language, t)

	// init input / output / error files
	outputCollector := runner.PipeCollector{Name: "stdout", SizeLimit: maxOutput}
	errorCollector := runner.PipeCollector{Name: "stderr", SizeLimit: maxOutput}

	// calculate time limits
	timeLimit := time.Duration(param.TimeLimit) * time.Millisecond
	if task.TimeLimit > 0 {
		timeLimit = time.Duration(task.TimeLimit) * time.Millisecond
	}
	wait := &waiter{timeLimit: timeLimit}

	memoryLimit := param.MemoryLimit << 10
	if task.MemoryLimit > 0 {
		memoryLimit = task.MemoryLimit << 10
	}

	// copyin files
	copyIn := make(map[string]file.File)
	// copyin source code for compile or exec files for exec
	if t == language.TypeCompile {
		copyIn[param.SourceFileName] = memfile.New("source", []byte(task.Code))
	} else {
		for _, f := range task.ExecFiles {
			copyIn[f.Name()] = f
		}
	}

	// copyout files: If compile read compiled files
	var copyOut []string
	if task.Type == "compile" {
		copyOut = param.CompiledFileNames
	}

	// build run specs
	c := &runner.Cmd{
		Args:        param.Args,
		Env:         param.Env,
		Files:       []interface{}{task.InputFile, outputCollector, errorCollector},
		MemoryLimit: memoryLimit,
		PidLimit:    param.ProcLimit,
		CopyIn:      copyIn,
		CopyOut:     copyOut,
		Waiter:      wait.Wait,
	}

	// run
	rn := &runner.Runner{
		CGBuilder:  r.CgroupBuilder,
		MasterPool: r.pool,
		Cmds:       []*runner.Cmd{c},
	}

	rt, err := rn.Run()
	if err != nil {
		return errResult(err.Error())
	}
	r0 := rt[0]

	inputContent, err := task.InputFile.Content()
	if err != nil {
		return errResult(err.Error())
	}
	userOutput, err := r0.Files["stdout"].Content()
	if err != nil {
		return errResult(err.Error())
	}
	userError, err := r0.Files["stderr"].Content()
	if err != nil {
		return errResult(err.Error())
	}

	// compile copyout
	var exec []file.File
	if task.Type == "compile" {
		for _, n := range param.CompiledFileNames {
			exec = append(exec, r0.Files[n])
		}
	}

	// TODO: diff

	return &types.RunTaskResult{
		Status:     r0.Status.String(),
		Time:       uint64(r0.Time / time.Millisecond),
		Memory:     r0.Memory >> 10,
		Input:      inputContent,
		UserOutput: userOutput,
		UserError:  userError,
		ExecFiles:  exec,
	}
}

func errResult(err string) *types.RunTaskResult {
	return &types.RunTaskResult{
		Status: "JGF",
		Error:  err,
	}
}

const minCPUPercent = 40 // 40%
const checkIntervalMS = 50

type waiter struct {
	timeLimit time.Duration
}

func (w *waiter) Wait(done chan struct{}, cg *cgroup.CGroup) bool {
	var lastCPUUsage uint64
	var totalTime time.Duration
	lastCheckTime := time.Now()
	// wait task done (check each interval)
	ticker := time.NewTicker(checkIntervalMS * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C: // interval
			cpuUsage, err := cg.CpuacctUsage()
			if err != nil {
				return true
			}

			cpuUsageDelta := time.Duration(cpuUsage - lastCPUUsage)
			timeDelta := now.Sub(lastCheckTime)

			totalTime += durationMax(cpuUsageDelta, timeDelta*minCPUPercent/100)
			if totalTime > w.timeLimit {
				return true
			}

			lastCheckTime = now
			lastCPUUsage = cpuUsage

		case <-done: // returned
			return false
		}
	}
}

func durationMax(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
