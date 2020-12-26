package macsandbox

import (
	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-sandbox/runner"
)

var _ envexec.Process = &process{}

type process struct {
	pid    int
	done   chan struct{}
	result runner.Result
}

func (p *process) Done() <-chan struct{} {
	return p.done
}

func (p *process) Result() runner.Result {
	<-p.done
	return p.result
}

func (p *process) Usage() envexec.Usage {
	return envexec.Usage{}
}
