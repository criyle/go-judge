package linuxcontainer

import (
	"fmt"
	"sync"
	"time"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-sandbox/runner"
)

var _ envexec.FreezableProcess = &process{}

// process defines the running process
type process struct {
	rt       runner.Result
	done     chan struct{}
	cg       Cgroup
	cpuStart time.Duration
	cgPool   CgroupPool
	release  sync.Once
}

func newProcess(run func() runner.Result, cg Cgroup, cgPool CgroupPool) *process {
	p := &process{
		done:   make(chan struct{}),
		cg:     cg,
		cgPool: cgPool,
	}
	if cg != nil {
		p.cpuStart, _ = cg.CPUUsage()
	}
	go func() {
		p.rt = run()
		p.collectUsage()
		close(p.done)
	}()
	return p
}

func (p *process) collectUsage() {
	if p.cg == nil {
		return
	}
	if t, err := p.cg.CPUUsage(); err == nil {
		p.rt.Time = durationDelta(t, p.cpuStart)
	}
	if m, err := p.cg.MaxMemory(); err == nil && m > 0 {
		p.rt.Memory = m
	}
	if pp, err := p.cg.ProcPeak(); err == nil && pp > 0 {
		p.rt.ProcPeak = pp
	}
}

func (p *process) Done() <-chan struct{} {
	return p.done
}

func (p *process) Result() envexec.RunnerResult {
	<-p.done
	p.release.Do(func() {
		if p.cgPool != nil && p.cg != nil {
			p.cgPool.Put(p.cg)
		}
	})
	return p.rt
}

func (p *process) Usage() envexec.Usage {
	var (
		t time.Duration
		m envexec.Size
	)
	if p.cg != nil {
		t, _ = p.cg.CPUUsage()
		t = durationDelta(t, p.cpuStart)
		m, _ = p.cg.CurrentMemory()
	}
	return envexec.Usage{
		Time:   t,
		Memory: m,
	}
}

func durationDelta(current, start time.Duration) time.Duration {
	if current < start {
		return 0
	}
	return current - start
}

func (p *process) Freeze() error {
	if p.cg == nil {
		return fmt.Errorf("freeze process: cgroup v2 is unavailable")
	}
	return p.cg.Freeze()
}

func (p *process) Resume() error {
	if p.cg == nil {
		return fmt.Errorf("resume process: cgroup v2 is unavailable")
	}
	return p.cg.Resume()
}
