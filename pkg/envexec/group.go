package envexec

import (
	"fmt"
	"sync"

	"github.com/criyle/go-sandbox/container"
)

// Group defines the running instruction to run multiple
// exec in parallel restricted within cgroup
type Group struct {
	// CgroupPool defines pool of cgroup used for Cmd
	CgroupPool CgroupPool

	// EnvironmentPool defines pool used for runner environment
	EnvironmentPool EnvironmentPool

	// Cmd defines Cmd running in parallel in multiple environments
	Cmd []*Cmd

	// Pipes defines the potential mapping between Cmd.
	// ensure nil is used as placeholder in correspond cmd
	Pipes []*Pipe
}

// PipeIndex defines the index of cmd and the fd of the that cmd
type PipeIndex struct {
	Index int
	Fd    int
}

// Pipe defines the pipe between parallel Cmd
type Pipe struct {
	In, Out PipeIndex
}

// Run starts the cmd and returns exec results
func (r *Group) Run() ([]Result, error) {
	// prepare files
	fds, pipeToCollect, fileToClose, err := prepareFds(r)
	defer func() { closeFiles(fileToClose) }()

	if err != nil {
		return nil, err
	}

	// prepare environments
	ms := make([]container.Environment, 0, len(r.Cmd))
	for range r.Cmd {
		m, err := r.EnvironmentPool.Get()
		if err != nil {
			return nil, fmt.Errorf("failed to get environment %v", err)
		}
		defer r.EnvironmentPool.Put(m)
		ms = append(ms, m)
	}

	// prepare cgroup
	cgs := make([]Cgroup, 0, len(r.Cmd))
	for range r.Cmd {
		cg, err := r.CgroupPool.Get()
		if err != nil {
			return nil, fmt.Errorf("failed to get cgroup %v", err)
		}
		defer r.CgroupPool.Put(cg)
		cgs = append(cgs, cg)
	}

	result := make([]Result, len(r.Cmd))
	errC := make(chan error, 1)

	// wait all cmd to finish
	var wg sync.WaitGroup
	wg.Add(len(r.Cmd))
	for i, c := range r.Cmd {
		go func(i int, c *Cmd) {
			defer wg.Done()
			r, err := runSingle(ms[i], cgs[i], c, fds[i], pipeToCollect[i])
			if err != nil {
				writeErrorC(errC, err)
				result[i].Status = StatusInternalError
				result[i].Error = err.Error()
				return
			}
			result[i] = r
		}(i, c)
	}
	wg.Wait()
	fileToClose = nil // already closed by runOne

	// collect potential error
	select {
	case err = <-errC:
	default:
	}
	return result, err
}
