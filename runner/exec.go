package runner

import (
	"bytes"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-judge/language"
	"github.com/criyle/go-judge/pkg/diff"
	"github.com/criyle/go-judge/pkg/envexec"
)

func (r *Runner) exec(done <-chan struct{}, task *ExecTask) *RunTaskResult {
	param := r.Language.Get(task.Exec.Language, language.TypeExec)

	execErr := func(status envexec.Status, err string) *RunTaskResult {
		return &RunTaskResult{
			Status: RunTaskFailed,
			Exec: &ExecResult{
				Status: status,
				Error:  err,
			},
		}
	}

	// init input / output / error files
	const stdout = "stdout"
	const stderr = "stderr"
	outputCollector := envexec.PipeCollector{Name: stdout, SizeLimit: maxOutput}
	errorCollector := envexec.PipeCollector{Name: stderr, SizeLimit: maxOutput}

	// calculate time limits
	timeLimit := param.TimeLimit
	if task.TimeLimit > 0 {
		timeLimit = task.TimeLimit
	}
	wait := &waiter{timeLimit: timeLimit}

	// calculate memory limits
	memoryLimit := param.MemoryLimit
	if task.MemoryLimit > 0 {
		memoryLimit = task.MemoryLimit
	}

	// copyin files
	copyIn := make(map[string]file.File)
	for _, f := range task.Exec.Exec {
		copyIn[f.Name()] = f
	}

	// copyout files
	var copyOut []string

	// build run specs
	c := &envexec.Cmd{
		Args:        param.Args,
		Env:         param.Env,
		Files:       []interface{}{task.InputFile, outputCollector, errorCollector},
		MemoryLimit: memoryLimit,
		ProcLimit:   param.ProcLimit,
		CopyIn:      copyIn,
		CopyOut:     copyOut,
		Waiter:      wait.Wait,
	}

	// run
	rn := &envexec.Single{
		CgroupPool:      r.cgPool,
		EnvironmentPool: r.pool,
		Cmd:             c,
	}

	rt, err := rn.Run()
	if err != nil {
		return execErr(envexec.StatusInternalError, err.Error())
	}

	// get result files
	inputContent, err := task.InputFile.Content()
	if err != nil {
		return execErr(envexec.StatusFileError, err.Error())
	}
	userOutput, err := getFile(rt.Files, stdout)
	if err != nil {
		return execErr(envexec.StatusFileError, err.Error())
	}
	userError, err := getFile(rt.Files, stderr)
	if err != nil {
		return execErr(envexec.StatusFileError, err.Error())
	}

	// compare result with answer (no spj now)
	var (
		status    = rt.Status
		spjOutput []byte
		scoreRate float64 = 1
	)
	ans, err := task.AnswerFile.Content()
	if err != nil {
		return execErr(envexec.StatusFileError, err.Error())
	}
	if status == envexec.StatusAccepted {
		if err := diff.Compare(bytes.NewReader(ans), bytes.NewReader(userOutput)); err != nil {
			spjOutput = []byte(err.Error())
			scoreRate = 0
			status = envexec.StatusWrongAnswer
		}
	}

	// return result
	return &RunTaskResult{
		Status: RunTaskSucceeded,
		Exec: &ExecResult{
			Status:      status,
			ScoringRate: scoreRate,
			Time:        rt.Time,
			Memory:      rt.Memory,
			Input:       inputContent,
			Answer:      ans,
			UserOutput:  userOutput,
			UserError:   userError,
			SPJOutput:   spjOutput,
		},
	}
}
