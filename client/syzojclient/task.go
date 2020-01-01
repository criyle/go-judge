package syzojclient

import (
	"log"

	"github.com/criyle/go-judge/client"
	"github.com/criyle/go-judge/types"
)

var _ client.Task = &Task{}

// Task task
type Task struct {
	client *Client
	task   *types.JudgeTask
	ackID  uint64

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
loop:
	for {
		select {
		case pConf := <-t.parsed:
			log.Println(pConf)

		case compiled := <-t.compiled:
			log.Println(compiled)

		case progressed := <-t.progressed:
			log.Println(progressed)

		case finished := <-t.finished:
			log.Println(finished)

			t.client.ack <- ack{id: t.ackID}
			t.client.request <- struct{}{}
			break loop
		}
	}
}
