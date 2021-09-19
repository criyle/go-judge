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
	Pipes []Pipe

	// NewStoreFile defines interface to create stored file
	NewStoreFile NewStoreFile
}

// PipeIndex defines the index of cmd and the fd of the that cmd
type PipeIndex struct {
	Index int
	Fd    int
}

// Pipe defines the pipe between parallel Cmd
type Pipe struct {
	// In, Out defines the pipe input source and output destination
	In, Out PipeIndex

	// Name defines copy out entry name if it is not empty and proxy is enabled
	Name string

	// Limit defines maximun bytes copy out from proxy and proxy will still
	// copy data after limit exceeded
	Limit Size

	// Proxy creates 2 pipe and connects them by copying data
	Proxy bool
}

// Run starts the cmd and returns exec results
func (r *Group) Run(ctx context.Context) ([]Result, error) {
	// prepare files
	fds, pipeToCollect, err := prepareFds(r, r.NewStoreFile)
	if err != nil {
		return nil, err
	}

	// wait all cmd to finish
	var g errgroup.Group
	result := make([]Result, len(r.Cmd))
	for i, c := range r.Cmd {
		i, c := i, c
		g.Go(func() error {
			r, err := runSingle(ctx, c, fds[i], pipeToCollect[i], r.NewStoreFile)
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
