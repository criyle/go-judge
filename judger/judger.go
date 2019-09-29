package judger

import (
	"github.com/criyle/go-judge/client"
	"github.com/criyle/go-judge/problem"
	"github.com/criyle/go-judge/taskqueue"
)

// Judger receives task from client and translate to task for runner
type Judger struct {
	client.Client
	taskqueue.Sender
	problem.Builder
}
