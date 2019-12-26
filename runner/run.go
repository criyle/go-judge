package runner

import (
	"bytes"
	"time"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-judge/file/memfile"
	"github.com/criyle/go-judge/language"
	"github.com/criyle/go-judge/pkg/diff"
	"github.com/criyle/go-judge/pkg/runner"
	"github.com/criyle/go-judge/types"
)

const maxOutput = 4 << 20 // 4M

func (r *Runner) run(done <-chan struct{}, task *types.RunTask) *types.RunTaskResult {
	t := language.TypeExec
	if task.Type == types.Compile {
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
		for _, f := range task.ExtraFiles {
			copyIn[f.Name()] = f
		}
	} else {
		for _, f := range task.ExecFiles {
			copyIn[f.Name()] = f
		}
	}

	// copyout files: If compile read compiled files
	var copyOut []string
	if task.Type == types.Compile {
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
		return errResult("JGF", err.Error())
	}
	r0 := rt[0]

	// get result files
	inputContent, err := task.InputFile.Content()
	if err != nil {
		return errResult("FileError", err.Error())
	}
	userOutput, err := r0.Files["stdout"].Content()
	if err != nil {
		return errResult("FileError", err.Error())
	}
	userError, err := r0.Files["stderr"].Content()
	if err != nil {
		return errResult("FileError", err.Error())
	}

	// compile copyout
	var exec []file.File
	if task.Type == types.Compile {
		for _, n := range param.CompiledFileNames {
			exec = append(exec, r0.Files[n])
		}
	}

	// compare result with answer (no spj now)
	var (
		status    = r0.Status.String()
		spjOutput []byte
		scoreRate float64 = 1
	)
	if task.Type != types.Compile {
		ans, err := task.AnswerFile.Content()
		if err != nil {
			return errResult("FileError", err.Error())
		}
		if err := diff.Compare(bytes.NewReader(ans), bytes.NewReader(userOutput)); err != nil {
			spjOutput = []byte(err.Error())
			scoreRate = 0
			status = "WA"
		}
	}

	// return result
	return &types.RunTaskResult{
		Status:      status,
		Time:        uint64(r0.Time / time.Millisecond),
		Memory:      r0.Memory >> 10,
		Input:       inputContent,
		UserOutput:  userOutput,
		UserError:   userError,
		SpjOutput:   spjOutput,
		ScoringRate: scoreRate,
		ExecFiles:   exec,
	}
}

func errResult(status, err string) *types.RunTaskResult {
	return &types.RunTaskResult{
		Status: status,
		Error:  err,
	}
}
