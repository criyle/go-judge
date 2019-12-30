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
}

// Param param
func (t *Task) Param() *types.JudgeTask {
	return t.task
}

// Parsed parsed
func (t *Task) Parsed(p *types.ProblemConfig) {
	//log.Println(p)
}

// Compiled compiled
func (t *Task) Compiled(p *types.ProgressCompiled) {
	log.Println(p)
}

// Progress progress
func (t *Task) Progress(p *types.ProgressProgressed) {
	log.Println(p)

}

// Finish finish
func (t *Task) Finish(r *types.JudgeResult) {
	log.Println(r)

	t.client.ack <- ack{id: t.ackID}
	t.client.request <- struct{}{}
}
