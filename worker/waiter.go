package worker

import (
	"context"
	"time"

	"github.com/criyle/go-judge/pkg/envexec"
)

const tickInterval = time.Second

type waiter struct {
	timeLimit     time.Duration
	realTimeLimit time.Duration
}

func (w *waiter) Wait(ctx context.Context, u envexec.Process) bool {
	if w.realTimeLimit < w.timeLimit {
		w.realTimeLimit = w.timeLimit
	}

	start := time.Now()

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false

		case <-ticker.C:
			if time.Since(start) > w.realTimeLimit {
				return true
			}
			u := u.Usage()
			if u.Time > w.timeLimit {
				return true
			}
		}
	}
}
