package envexec

import (
	"fmt"

	"golang.org/x/sync/errgroup"
)

// Group defines the running instruction to run multiple
// exec in parallel restricted within cgroup
type Group struct {
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
	ms := make([]Environment, 0, len(r.Cmd))
	for range r.Cmd {
		m, err := r.EnvironmentPool.Get()
		if err != nil {
			return nil, fmt.Errorf("failed to get environment %v", err)
		}
		defer r.EnvironmentPool.Put(m)
		ms = append(ms, m)
	}

	// wait all cmd to finish
	var g errgroup.Group
	result := make([]Result, len(r.Cmd))
	for i, c := range r.Cmd {
		i, c := i, c
		g.Go(func() error {
			r, err := runSingle(ms[i], c, fds[i], pipeToCollect[i])
			result[i] = r
			if err != nil {
				result[i].Status = StatusInternalError
				result[i].Error = err.Error()
				return err
			}
			return nil
		})
	}
	err = g.Wait()
	return result, err
}
