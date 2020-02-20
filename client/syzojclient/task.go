package syzojclient

import (
	"log"
	"time"

	"github.com/criyle/go-judge/client"
	"github.com/criyle/go-judge/types"
	"github.com/ugorji/go/codec"
)

var _ client.Task = &Task{}

// Task task
type Task struct {
	client *Client
	task   *types.JudgeTask
	ackID  uint64
	taskID string

	parsed     chan *types.ProblemConfig
	compiled   chan *types.ProgressCompiled
	progressed chan *types.ProgressProgressed
	finished   chan *types.JudgeResult
}

// Param param
func (t *Task) Param() *types.JudgeTask {
	return t.task
}

// Parsed parsed
func (t *Task) Parsed(p *types.ProblemConfig) {
	t.parsed <- p
}

// Compiled compiled
func (t *Task) Compiled(p *types.ProgressCompiled) {
	t.compiled <- p
}

// Progressed progress
func (t *Task) Progressed(p *types.ProgressProgressed) {
	t.progressed <- p
}

// Finished finished
func (t *Task) Finished(r *types.JudgeResult) {
	t.finished <- r
}

func (t *Task) loop() {
	var (
		jr judgeResult
		cr *compileResult
	)
	encode := func(p interface{}) []byte {
		var d []byte
		if err := codec.NewEncoderBytes(&d, &codec.MsgpackHandle{}).Encode(p); err != nil {
			// handler error (unlikely)
			return d
		}
		return d
	}
loop:
	for {
		select {
		case pConf := <-t.parsed:
			log.Println(pConf)
			initResult(pConf, &jr)
			rt := result{
				TaskID: t.taskID,
				Type:   progressStarted,
			}
			t.client.progress <- encode(rt)

		case compiled := <-t.compiled:
			log.Println(compiled)
			cr = &compileResult{
				Status:  convertStatus(compiled.Status),
				Message: compiled.Message,
			}
			rt := result{
				TaskID: t.taskID,
				Type:   progressCompiled,
				Progress: progress{
					Status:  convertStatus(compiled.Status),
					Message: compiled.Message,
				},
			}
			t.client.progress <- encode(rt)
			t.client.result <- encode(rt)

		case progressed := <-t.progressed:
			log.Println(progressed)
			updateResult(progressed, &jr)
			rt := result{
				TaskID: t.taskID,
				Type:   progressProgress,
				Progress: progress{
					Compile: cr,
					Judge:   &jr,
				},
			}
			t.client.progress <- encode(rt)

		case finished := <-t.finished:
			log.Println(finished)

			rt := result{
				TaskID: t.taskID,
				Type:   progressFinished,
				Progress: progress{
					Compile: cr,
					Judge:   &jr,
				},
			}
			t.client.progress <- encode(rt)
			t.client.result <- encode(rt)

			t.client.ack <- ack{id: t.ackID}
			t.client.request <- struct{}{}
			break loop
		}
	}
}

func initResult(p *types.ProblemConfig, jr *judgeResult) {
	jr.Subtasks = make([]subtaskResult, len(p.Subtasks))
	for i := range jr.Subtasks {
		initSubtaskResult(&p.Subtasks[i], &jr.Subtasks[i])
	}
}

func initSubtaskResult(p *types.SubTask, sr *subtaskResult) {
	sr.Cases = make([]caseResult, len(p.Cases))
}

func convertStatus(s types.ProgressStatus) taskStatus {
	switch s {
	case types.ProgressSucceeded:
		return statusDone
	default:
		return statusFailed
	}
}

func convertResultTypes(s types.Status) testCaseResultType {
	switch s {
	case types.StatusAccepted:
		return resultAccepted
	case types.StatusWrongAnswer:
		return resultWrongAnswer
	case types.StatusPartiallyCorrect:
		return resultPartiallyCorrect
	case types.StatusMemoryLimitExceeded:
		return resultMemoryLimitExceeded
	case types.StatusTimeLimitExceeded:
		return resultTimeLimitExceeded
	case types.StatusOutputLimitExceeded:
		return resultOutputLimitExceeded
	case types.StatusFileError:
		return resultFileError
	case types.StatusRuntimeError:
		return resultRuntimeError
	case types.StatusJudgementFailed:
		return resultJudgementFailed
	case types.StatusInvalidInteraction:
		return resultInvalidInteraction
	default:
		return resultRuntimeError
	}
}

func updateResult(p *types.ProgressProgressed, jr *judgeResult) {
	st := &jr.Subtasks[p.SubTaskIndex]
	st.Score += 100 * p.ScoreRate / float64(len(jr.Subtasks[p.SubTaskIndex].Cases))

	cr := &st.Cases[p.TestCaseIndex]
	cr.Status = convertStatus(p.Status)
	cr.Error = p.Error
	cr.Result = &testcaseDetails{
		Status:      convertResultTypes(p.ExecStatus),
		Time:        uint64(p.Time / time.Millisecond),
		Memory:      uint64(p.Memory >> 10),
		Input:       getFileContent("input", p.Input),
		Output:      getFileContent("output", p.Answer),
		ScoringRate: p.ScoreRate,
		UserOutput:  getStringP(p.UserOutput),
		UserError:   getStringP(p.UserError),
		SPJMessage:  getStringP(p.SPJOutput),
	}
}

func getFileContent(name string, b []byte) *fileContent {
	return &fileContent{
		Name:    name,
		Content: string(b),
	}
}

func getStringP(b []byte) *string {
	s := string(b)
	return &s
}
