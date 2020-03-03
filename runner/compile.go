package runner

import (
	"os"
	"time"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-judge/language"
	"github.com/criyle/go-judge/pkg/envexec"
)

func (r *Runner) compile(done <-chan struct{}, task *CompileTask) *RunTaskResult {
	param := r.Language.Get(task.Language, language.TypeCompile)

	compileErr := func(err string) *RunTaskResult {
		return &RunTaskResult{
			Status: RunTaskFailed,
			Compile: &CompileResult{
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
	copyIn[param.SourceFileName] = file.NewMemFile("source", source)
	for _, f := range task.ExtraFiles {
		copyIn[f.Name()] = f
	}

	// copyout files: If compile read compiled files
	var copyOut []string
	copyOut = param.CompiledFileNames

	// compile message (stdout & stderr)
	const msgFileName = "msg"
	msgCollector := envexec.PipeCollector{Name: msgFileName, SizeLimit: maxOutput}
	devNull := file.NewLocalFile("null", os.DevNull)

	// time limit
	wait := &waiter{timeLimit: time.Duration(param.TimeLimit)}

	// build run specs
	c := &envexec.Cmd{
		Args:        param.Args,
		Env:         param.Env,
		Files:       []interface{}{devNull, msgCollector, msgCollector},
		MemoryLimit: param.MemoryLimit,
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
		return compileErr(err.Error())
	}

	// get compile message
	compileMsg, err := getFile(rt.Files, msgFileName)
	if err != nil {
		return compileErr("FileError:" + err.Error())
	}

	// compile copyout
	var exec []file.File
	for _, n := range param.CompiledFileNames {
		f, err := getFile(rt.Files, n)
		if err != nil {
			return compileErr(string(compileMsg))
		}
		exec = append(exec, file.NewMemFile(n, f))
	}

	// return result
	return &RunTaskResult{
		Status: RunTaskSucceeded,
		Compile: &CompileResult{
			Exec: &file.CompiledExec{
				Language: task.Language,
				Exec:     exec,
			},
			Error: string(compileMsg),
		},
	}
}
