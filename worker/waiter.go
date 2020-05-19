package worker

import (
	"context"
	"time"

	"github.com/criyle/go-judge/pkg/envexec"
)

type waiter struct {
	timeLimit     time.Duration
	realTimeLimit time.Duration
}

func (w *waiter) Wait(ctx context.Context, u envexec.Process) bool {
	if w.realTimeLimit < w.timeLimit {
		w.realTimeLimit = w.timeLimit
	}

	timer := time.NewTimer(w.realTimeLimit)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
