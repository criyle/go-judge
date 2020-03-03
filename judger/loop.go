package judger

import (
	"fmt"
	"sync"

	"github.com/criyle/go-judge/client"
	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-judge/problem"
	"github.com/criyle/go-judge/runner"
)

// Loop fetch judge task from client and report results
// in a infinite loop
func (j *Judger) Loop(done <-chan struct{}) {
	c := j.Client.C()
loop:
	for {
		select {
		case t := <-c:
			rt := j.run(done, t)
			t.Finished(rt)

			select {
			case <-done:
				break loop
			default:
			}
		case <-done:
			break loop
		}
	}
}

func (j *Judger) run(done <-chan struct{}, t client.Task) *client.JudgeResult {
	var result client.JudgeResult
	errResult := func(err error) *client.JudgeResult {
		result.Error = err.Error()
		return &result
	}
	errResultF := func(f string, v ...interface{}) *client.JudgeResult {
		result.Error = fmt.Sprintf(f, v...)
		return &result
	}

	p := t.Param()
	pConf, err := j.Build(p.TestData)
	if err != nil {
		return errResult(err)
	}

	// parsed
	t.Parsed(&pConf)

	// compile
	compileRet, err := j.Send(runner.RunTask{
		Type:    problem.Compile,
		Compile: (*runner.CompileTask)(&p.Code),
	})
	if err != nil {
		return errResult(err)
	}
	compileTaskResult := <-compileRet

	// compiled
	if compileTaskResult.Compile == nil {
		t.Compiled(&client.ProgressCompiled{
			Status: client.ProgressFailed,
		})
		return errResultF("compile error: no response")
	}
	if compileTaskResult.Status != runner.RunTaskSucceeded {
		t.Compiled(&client.ProgressCompiled{
			Status:  client.ProgressFailed,
			Message: compileTaskResult.Compile.Error,
		})
		return errResultF("compile error: %s", compileTaskResult.Compile.Error)
	}
	t.Compiled(&client.ProgressCompiled{
		Status: client.ProgressSucceeded,
	})

	// judger
	pj := problemJudger{
		Judger:    j,
		Config:    &pConf,
		Task:      t,
		JudgeTask: p,
		Exec:      compileTaskResult.Compile.Exec,
	}

	// run all subtasks
	var wg sync.WaitGroup
	wg.Add(len(pConf.Subtasks))

	result.SubTasks = make([]client.SubTaskResult, len(pConf.Subtasks))
	for i := range pConf.Subtasks {
		go func(index int) {
			defer wg.Done()

			s := &pConf.Subtasks[index]
			result.SubTasks[index] = pj.runSubtask(done, s, index)
		}(i)
	}
	wg.Wait()

	return &result
}

type problemJudger struct {
	*Judger
	*problem.Config
	*client.JudgeTask
	client.Task

	// compiled code
	Exec *file.CompiledExec
}

func (pj *problemJudger) runSubtask(done <-chan struct{}, s *problem.SubTask, sIndex int) client.SubTaskResult {
	var result client.SubTaskResult
	result.Cases = make([]client.TestCaseResult, len(s.Cases))

	// wait for all cases
	var wg sync.WaitGroup
	wg.Add(len(s.Cases))

	for i := range s.Cases {
		go func(i int) {
			defer wg.Done()

			c := s.Cases[i]

			rtC, err := pj.Send(runner.RunTask{
				Type: pj.Config.Type,
				Exec: &runner.ExecTask{
					Exec:        pj.Exec,
					TimeLimit:   pj.TimeLimit,
					MemoryLimit: pj.MemoryLimit,
					InputFile:   c.Input,
					AnswerFile:  c.Answer,
				},
			})

			var ret client.TestCaseResult
			if err != nil {
				ret.Status = client.ProgressFailed
			} else {
				// receive result from queue
				rt := <-rtC

				// run task result -> test case result
				ret.Status = client.ProgressStatus(rt.Status)
				if execRt := rt.Exec; execRt != nil {
					ret.ExecStatus = execRt.Status
					ret.Error = execRt.Error
					ret.ScoreRate = execRt.ScoringRate
					ret.Time = execRt.Time
					ret.Memory = execRt.Memory
					ret.Input = execRt.Input
					ret.Answer = execRt.Answer
					ret.UserOutput = execRt.UserOutput
					ret.UserError = execRt.UserError
					ret.SPJOutput = execRt.SPJOutput
				}
			}

			// store result
			result.Cases[i] = ret

			// report prograss
			pj.Progressed(&client.ProgressProgressed{
				SubTaskIndex:   sIndex,
				TestCaseIndex:  i,
				TestCaseResult: ret,
			})
		}(i)
	}
	wg.Wait()

	// calculate score
	for _, r := range result.Cases {
		result.Score += r.ScoreRate
	}

	return result
}

// count counts total number of cases
func count(pConf *problem.Config) int32 {
	var count int32
	for _, s := range pConf.Subtasks {
		count += int32(len(s.Cases))
	}
	return count
}
