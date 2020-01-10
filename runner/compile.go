package runner

import (
	"os"
	"time"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-judge/file/localfile"
	"github.com/criyle/go-judge/file/memfile"
	"github.com/criyle/go-judge/language"
	"github.com/criyle/go-judge/pkg/runner"
	"github.com/criyle/go-judge/types"
)

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
		return compileErr("FileError:" + err.Error())
	}

	// compile copyout
	var exec []file.File
	for _, n := range param.CompiledFileNames {
		f, err := getFile(r0.Files, n)
		if err != nil {
			return compileErr(string(compileMsg))
		}
		exec = append(exec, memfile.New(n, f))
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
