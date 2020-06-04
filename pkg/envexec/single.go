package envexec

import (
	"context"
	"fmt"
)

// Single defines the running instruction to run single
// exec in restricted within cgroup
type Single struct {
	// EnvironmentPool defines pool used for runner environment
	EnvironmentPool EnvironmentPool

	// Cmd defines Cmd running in parallel in multiple environments
	Cmd *Cmd
}

// Run starts the cmd and returns exec results
func (s *Single) Run(ctx context.Context) (result Result, err error) {
	// prepare files
	fd, fileToClose, pipeToCollect, err := prepareCmdFd(s.Cmd, len(s.Cmd.Files))
	defer func() { closeFiles(fileToClose) }()

	if err != nil {
		return result, err
	}

	// prepare environment
	m, err := s.EnvironmentPool.Get()
	if err != nil {
		return result, fmt.Errorf("failed to get environment %v", err)
	}
	defer s.EnvironmentPool.Put(m)

	result, err = runSingle(ctx, m, s.Cmd, fd, pipeToCollect)
	fileToClose = nil // already closed by runOne
	if err != nil {
		result.Status = StatusInternalError
		result.Error = err.Error()
		return result, err
	}

	// collect potential error
	return result, err
}
