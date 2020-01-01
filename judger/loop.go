package judger

import (
	"fmt"
	"sync"

	"github.com/criyle/go-judge/client"
	"github.com/criyle/go-judge/types"
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

func (j *Judger) run(done <-chan struct{}, t client.Task) *types.JudgeResult {
	var result types.JudgeResult
	errResult := func(err error) *types.JudgeResult {
		result.Error = err.Error()
		return &result
	}
	errResultF := func(f string, v ...interface{}) *types.JudgeResult {
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
	compileRet, err := j.Send(types.RunTask{
		Type:    types.Compile,
		Compile: (*types.CompileTask)(&p.Code),
	})
	if err != nil {
		return errResult(err)
	}
	compileTaskResult := <-compileRet

	// compiled
	if compileTaskResult.Compile == nil {
		t.Compiled(&types.ProgressCompiled{
			Status: types.ProgressFailed,
		})
		return errResultF("compile error: no response")
	}
	if compileTaskResult.Status != types.RunTaskSucceeded {
		t.Compiled(&types.ProgressCompiled{
			Status:  types.ProgressFailed,
			Message: compileTaskResult.Compile.Error,
		})
		return errResultF("compile error: %s", compileTaskResult.Compile.Error)
	}
	t.Compiled(&types.ProgressCompiled{
		Status: types.ProgressSucceeded,
	})

	// judger
	pj := problemJudger{
		Judger:        j,
		ProblemConfig: &pConf,
		Task:          t,
		JudgeTask:     p,
		Exec:          compileTaskResult.Compile.Exec,
	}

	// run all subtasks
	var wg sync.WaitGroup
	wg.Add(len(pConf.Subtasks))

	result.SubTasks = make([]types.SubTaskResult, len(pConf.Subtasks))
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
	*types.ProblemConfig
	*types.JudgeTask
	client.Task

	// compiled code
	Exec *types.CompiledExec
}

func (pj *problemJudger) runSubtask(done <-chan struct{}, s *types.SubTask, sIndex int) types.SubTaskResult {
	var result types.SubTaskResult
	result.Cases = make([]types.TestCaseResult, len(s.Cases))

	// wait for all cases
	var wg sync.WaitGroup
	wg.Add(len(s.Cases))

	for i := range s.Cases {
		go func(i int) {
			defer wg.Done()

			c := s.Cases[i]

			rtC, err := pj.Send(types.RunTask{
				Type: pj.ProblemConfig.Type,
				Exec: &types.ExecTask{
					Exec:        pj.Exec,
					TimeLimit:   pj.TileLimit,
					MemoryLimit: pj.MemoryLimit,
					InputFile:   c.Input,
					AnswerFile:  c.Answer,
				},
			})

			var ret types.TestCaseResult
			if err != nil {
				ret.Status = types.ProgressFailed
			} else {
				// receive result from queue
				rt := <-rtC

				// run task result -> test case result
				ret.Status = types.ProgressStatus(rt.Status)
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
			pj.Progressed(&types.ProgressProgressed{
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
func count(pConf *types.ProblemConfig) int32 {
	var count int32
	for _, s := range pConf.Subtasks {
		count += int32(len(s.Cases))
	}
	return count
}
