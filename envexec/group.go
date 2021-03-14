package envexec

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// Group defines the running instruction to run multiple
// exec in parallel restricted within cgroup
type Group struct {
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
func (r *Group) Run(ctx context.Context) ([]Result, error) {
	// prepare files
	fds, pipeToCollect, err := prepareFds(r)
	if err != nil {
		return nil, err
	}

	// wait all cmd to finish
	var g errgroup.Group
	result := make([]Result, len(r.Cmd))
	for i, c := range r.Cmd {
		i, c := i, c
		g.Go(func() error {
			r, err := runSingle(ctx, c, fds[i], pipeToCollect[i])
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
