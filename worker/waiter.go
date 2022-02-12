package worker

import (
	"context"
	"time"

	"github.com/criyle/go-judge/envexec"
)

// default tick interval 100 ms
const defaultTickInterval = 100 * time.Millisecond

type waiter struct {
	tickInterval   time.Duration
	timeLimit      time.Duration
	clockTimeLimit time.Duration
}

func (w *waiter) Wait(ctx context.Context, u envexec.Process) bool {
	if w.clockTimeLimit < w.timeLimit {
		w.clockTimeLimit = w.timeLimit
	}

	start := time.Now()

	tickInterval := w.tickInterval
	if tickInterval == 0 {
		tickInterval = defaultTickInterval
	}

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false

		case <-u.Done():
			return false

		case <-ticker.C:
			if time.Since(start) > w.clockTimeLimit {
				return true
			}
			u := u.Usage()
			if u.Time > w.timeLimit {
				return true
			}
		}
	}
}
