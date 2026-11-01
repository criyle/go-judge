package envexec

import (
	"context"
	"sort"

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

	// CPUSet pins the proxy relay thread to the same cpuset as the commands.
	CPUSet string

	// Name defines copy out entry name if it is not empty and proxy is enabled
	Name string

	// Limit defines maximum bytes copy out from proxy and proxy will still
	// copy data after limit exceeded
	Limit Size

	// Proxy creates 2 pipe and connects them by copying data
	Proxy bool

	// Disable no copy on Linux
	DisableZeroCopy bool
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
	type exitEvent struct {
		index int
		at    int64
		seq   int
	}
	exitEvents := make(chan exitEvent, len(r.Cmd))
	for i, c := range r.Cmd {
		g.Go(func() error {
			r, err := runSingle(ctx, c, fds[i], pipeToCollect[i], r.NewStoreFile, func(at int64) {
				exitEvents <- exitEvent{index: i, at: at}
			})
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
	close(exitEvents)
	events := make([]exitEvent, 0, len(r.Cmd))
	for event := range exitEvents {
		event.seq = len(events)
		events = append(events, event)
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].at != events[j].at {
			return events[i].at < events[j].at
		}
		return events[i].seq < events[j].seq
	})
	for order, event := range events {
		result[event.index].ExitOrder = order + 1
	}
	return result, err
}
