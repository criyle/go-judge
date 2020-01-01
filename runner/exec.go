package runner

import (
	"bytes"
	"time"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-judge/language"
	"github.com/criyle/go-judge/pkg/diff"
	"github.com/criyle/go-judge/pkg/runner"
	"github.com/criyle/go-judge/types"
)

func (r *Runner) exec(done <-chan struct{}, task *types.ExecTask) *types.RunTaskResult {
	param := r.Language.Get(task.Exec.Language, language.TypeExec)

	execErr := func(status types.Status, err string) *types.RunTaskResult {
		return &types.RunTaskResult{
			Status: types.RunTaskFailed,
			Exec: &types.ExecResult{
				Status: status,
				Error:  err,
			},
		}
	}

	// init input / output / error files
	const stdout = "stdout"
	const stderr = "stderr"
	outputCollector := runner.PipeCollector{Name: stdout, SizeLimit: maxOutput}
	errorCollector := runner.PipeCollector{Name: stderr, SizeLimit: maxOutput}

	// calculate time limits
	timeLimit := time.Duration(param.TimeLimit) * time.Millisecond
	if task.TimeLimit > 0 {
		timeLimit = time.Duration(task.TimeLimit) * time.Millisecond
	}
	wait := &waiter{timeLimit: timeLimit}

	// calculate memory limits
	memoryLimit := param.MemoryLimit << 10
	if task.MemoryLimit > 0 {
		memoryLimit = task.MemoryLimit << 10
	}

	// copyin files
	copyIn := make(map[string]file.File)
	for _, f := range task.Exec.Exec {
		copyIn[f.Name()] = f
	}

	// copyout files
	var copyOut []string

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
		return execErr(types.StatusInternalError, err.Error())
	}
	r0 := rt[0]

	// get result files
	inputContent, err := task.InputFile.Content()
	if err != nil {
		return execErr(types.StatusFileError, err.Error())
	}
	userOutput, err := getFile(r0.Files, stdout)
	if err != nil {
		return execErr(types.StatusFileError, err.Error())
	}
	userError, err := getFile(r0.Files, stderr)
	if err != nil {
		return execErr(types.StatusFileError, err.Error())
	}

	// compare result with answer (no spj now)
	var (
		status    = r0.Status.String()
		spjOutput []byte
		scoreRate float64 = 1
	)
	ans, err := task.AnswerFile.Content()
	if err != nil {
		return execErr(types.StatusFileError, err.Error())
	}
	if err := diff.Compare(bytes.NewReader(ans), bytes.NewReader(userOutput)); err != nil {
		spjOutput = []byte(err.Error())
		scoreRate = 0
		status = "WA"
	}

	// return result
	return &types.RunTaskResult{
		Status: types.RunTaskSucceeded,
		Exec: &types.ExecResult{
			ScoringRate: scoreRate,
			Error:       status,
			Time:        uint64(r0.Time / time.Millisecond),
			Memory:      r0.Memory >> 10,
			Input:       inputContent,
			UserOutput:  userOutput,
			UserError:   userError,
			SPJOutput:   spjOutput,
		},
	}
}
