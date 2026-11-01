package macsandbox

import (
	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-sandbox/runner"
)

var _ envexec.Process = &process{}

type process struct {
	pid        int
	done       chan struct{}
	result     runner.Result
	finishedAt int64
}

func (p *process) Done() <-chan struct{} {
	return p.done
}

func (p *process) Result() runner.Result {
	<-p.done
	return p.result
}

func (p *process) FinishedAt() int64 {
	<-p.done
	return p.finishedAt
}

func (p *process) Usage() envexec.Usage {
	return envexec.Usage{}
}
