package runner

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-judge/file/localfile"
	"github.com/criyle/go-judge/file/memfile"
	"github.com/criyle/go-judge/language"
	"github.com/criyle/go-judge/pkg/diff"
	"github.com/criyle/go-judge/pkg/runner"
	"github.com/criyle/go-judge/types"
)

const maxOutput = 4 << 20 // 4M

func (r *Runner) compile(done <-chan struct{}, task *types.CompileTask) *types.RunTaskResult {
	param := r.Language.Get(task.Language, language.TypeCompile)

	compileErr := func(err string) *types.RunTaskResult {
		return &types.RunTaskResult{
			Status: types.RunTaskFailed,
			Compile: &types.CompileResult{
				Error: err,
			},
		}
	}

	// source code
	source, err := task.Code.Content()
	if err != nil {
		return compileErr("File Error")
	}

	// copyin files
	copyIn := make(map[string]file.File)
	copyIn[param.SourceFileName] = memfile.New("source", source)
	for _, f := range task.ExtraFiles {
		copyIn[f.Name()] = f
	}

	// copyout files: If compile read compiled files
	var copyOut []string
	copyOut = param.CompiledFileNames

	// compile message (stdout & stderr)
	const msgFileName = "msg"
	msgCollector := runner.PipeCollector{Name: msgFileName, SizeLimit: maxOutput}
	devNull := localfile.New("null", os.DevNull)

	// time limit
	wait := &waiter{timeLimit: time.Duration(param.TimeLimit) * time.Millisecond}

	// build run specs
	c := &runner.Cmd{
		Args:        param.Args,
		Env:         param.Env,
		Files:       []interface{}{devNull, msgCollector, msgCollector},
		MemoryLimit: param.MemoryLimit << 10,
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
		return compileErr(err.Error())
	}
	r0 := rt[0]

	// get compile message
	compileMsg, err := getFile(r0.Files, msgFileName)
	if err != nil {
		return compileErr("FileError" + err.Error())
	}

	// compile copyout
	var exec []file.File
	for _, n := range param.CompiledFileNames {
		exec = append(exec, r0.Files[n])
	}

	// return result
	return &types.RunTaskResult{
		Status: types.RunTaskSucceeded,
		Compile: &types.CompileResult{
			Exec: &types.CompiledExec{
				Language: task.Language,
				Exec:     exec,
			},
			Error: string(compileMsg),
		},
	}
}

func (r *Runner) exec(done <-chan struct{}, task *types.ExecTask) *types.RunTaskResult {
	param := r.Language.Get(task.Exec.Language, language.TypeExec)

	execErr := func(err string) *types.RunTaskResult {
		return &types.RunTaskResult{
			Status: types.RunTaskFailed,
			Exec: &types.ExecResult{
				Error: err,
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
		return execErr("JGF" + err.Error())
	}
	r0 := rt[0]

	// get result files
	inputContent, err := task.InputFile.Content()
	if err != nil {
		return execErr("FileError" + err.Error())
	}
	userOutput, err := getFile(r0.Files, stdout)
	if err != nil {
		return execErr("FileError" + err.Error())
	}
	userError, err := getFile(r0.Files, stderr)
	if err != nil {
		return execErr("FileError" + err.Error())
	}

	// compare result with answer (no spj now)
	var (
		status    = r0.Status.String()
		spjOutput []byte
		scoreRate float64 = 1
	)
	ans, err := task.AnswerFile.Content()
	if err != nil {
		return execErr("FileError" + err.Error())
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

func (r *Runner) run(done <-chan struct{}, task *types.RunTask) *types.RunTaskResult {
	switch task.Type {
	case types.Compile:
		return r.compile(done, task.Compile)

	default:
		return r.exec(done, task.Exec)
	}
}

func getFile(files map[string]file.File, name string) ([]byte, error) {
	if f, ok := files[name]; ok {
		return f.Content()
	}
	return nil, fmt.Errorf("file %s not exists", name)
}
