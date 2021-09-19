package envexec

import (
	"context"
)

// Single defines the running instruction to run single
// exec in restricted within cgroup
type Single struct {
	// Cmd defines Cmd running in parallel in multiple environments
	Cmd *Cmd

	// NewStoreFile defines interface to create stored file
	NewStoreFile NewStoreFile
}

// Run starts the cmd and returns exec results
func (s *Single) Run(ctx context.Context) (result Result, err error) {
	// prepare files
	fd, pipeToCollect, err := prepareCmdFd(s.Cmd, len(s.Cmd.Files), s.NewStoreFile)
	if err != nil {
		return result, err
	}

	result, err = runSingle(ctx, s.Cmd, fd, pipeToCollect, s.NewStoreFile)
	if err != nil {
		result.Status = StatusInternalError
		result.Error = err.Error()
		return result, err
	}

	// collect potential error
	return result, err
}
