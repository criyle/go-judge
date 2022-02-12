package linuxcontainer

import (
	"time"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-sandbox/runner"
)

var _ envexec.Process = &process{}

// process defines the running process
type process struct {
	rt   runner.Result
	done chan struct{}
	cg   Cgroup
}

func newProcess(run func() runner.Result, cg Cgroup, cgPool CgroupPool) *process {
	p := &process{
		done: make(chan struct{}),
		cg:   cg,
	}
	go func() {
		defer close(p.done)
		if cgPool != nil {
			defer cgPool.Put(cg)
		}
		p.rt = run()
		p.collectUsage()
	}()
	return p
}

func (p *process) collectUsage() {
	if p.cg == nil {
		return
	}
	if t, err := p.cg.CPUUsage(); err == nil {
		p.rt.Time = t
	}
	if m, err := p.cg.MaxMemory(); err == nil && m > 0 {
		p.rt.Memory = m
	}
}

func (p *process) Done() <-chan struct{} {
	return p.done
}

func (p *process) Result() envexec.RunnerResult {
	<-p.done
	return p.rt
}

func (p *process) Usage() envexec.Usage {
	var (
		t time.Duration
		m envexec.Size
	)
	if p.cg != nil {
		t, _ = p.cg.CPUUsage()
		m, _ = p.cg.CurrentMemory()
	}
	return envexec.Usage{
		Time:   t,
		Memory: m,
	}
}
