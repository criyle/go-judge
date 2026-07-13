package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/criyle/go-judge/envexec"
)

// default tick interval 100 ms
const defaultTickInterval = 100 * time.Millisecond

type waiter struct {
	tickInterval   time.Duration
	timeLimit      time.Duration
	clockTimeLimit time.Duration
	control        *RuntimeControl
}

type turnRuntimeState struct {
	active        bool
	turnID        uint64
	processStart  time.Duration
	turnCPUStart  time.Duration
	turnWallStart time.Time
	moveLimit     time.Duration
	totalLimit    time.Duration
	wallLimit     time.Duration
}

func (w *waiter) Wait(ctx context.Context, process envexec.Process) bool {
	if w.control == nil {
		return w.waitOrdinary(ctx, process)
	}
	return w.waitControlled(ctx, process)
}

func (w *waiter) waitOrdinary(ctx context.Context, process envexec.Process) bool {
	clockTimeLimit := w.clockTimeLimit
	if clockTimeLimit == 0 {
		clockTimeLimit = w.timeLimit
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
		case <-process.Done():
			return false
		case <-ticker.C:
			if time.Since(start) > clockTimeLimit || process.Usage().Time > w.timeLimit {
				return true
			}
		}
	}
}

func (w *waiter) waitControlled(ctx context.Context, process envexec.Process) bool {
	control := w.control
	defer close(control.stopped)
	freezable, supported := process.(envexec.FreezableProcess)
	state := turnRuntimeState{}
	ordinaryStart := time.Now()
	controlled := false

	ordinaryInterval := w.tickInterval
	if ordinaryInterval == 0 {
		ordinaryInterval = defaultTickInterval
	}
	ordinaryTicker := time.NewTicker(ordinaryInterval)
	defer ordinaryTicker.Stop()
	turnTicker := time.NewTicker(defaultTurnTickInterval)
	turnTicker.Stop()
	var turnTick <-chan time.Time
	defer turnTicker.Stop()

	emit := func(event TurnEvent) {
		control.publish(event)
	}
	fatal := func(event TurnEvent) bool {
		emit(event)
		if control.fatal != nil {
			control.fatal()
		}
		return true
	}
	usageEvent := func(now time.Time, typ TurnEventType) TurnEvent {
		current := process.Usage().Time
		return TurnEvent{
			TurnID:   state.turnID,
			Type:     typ,
			MoveCPU:  current - state.turnCPUStart,
			TotalCPU: current - state.processStart,
			WallTime: now.Sub(state.turnWallStart),
		}
	}
	checkLimits := func(now time.Time) (TurnEvent, bool) {
		event := usageEvent(now, TurnCompleted)
		switch {
		case state.moveLimit > 0 && event.MoveCPU > state.moveLimit:
			event.Type = MoveCPULimitExceeded
		case state.totalLimit > 0 && event.TotalCPU > state.totalLimit:
			event.Type = TotalCPULimitExceeded
		case state.wallLimit > 0 && event.WallTime > state.wallLimit:
			event.Type = MoveWallLimitExceeded
		default:
			return event, false
		}
		return event, true
	}
	freezeAndCheck := func(now time.Time, typ TurnEventType) (TurnEvent, error) {
		if !supported {
			return TurnEvent{}, fmt.Errorf("turn control is unsupported: process is not freezable")
		}
		if err := freezable.Freeze(); err != nil {
			return TurnEvent{}, fmt.Errorf("freeze process: %w", err)
		}
		event, exceeded := checkLimits(now)
		if exceeded {
			return event, nil
		}
		event.Type = typ
		return event, nil
	}

	for {
		select {
		case <-ctx.Done():
			return false
		case <-process.Done():
			if controlled {
				event := usageEvent(time.Now(), ProcessExited)
				return fatal(event)
			}
			return false
		case command := <-control.Commands:
			var replyErr error
			switch command.Type {
			case turnFreeze:
				controlled = true
				if !supported {
					replyErr = fmt.Errorf("turn control is unsupported: process is not freezable")
				} else {
					replyErr = freezable.Freeze()
				}
			case turnBegin:
				controlled = true
				if state.active {
					replyErr = fmt.Errorf("turn %d is still active", state.turnID)
				} else if !supported {
					replyErr = fmt.Errorf("turn control is unsupported: process is not freezable")
				} else {
					current := process.Usage().Time
					state.active = true
					state.turnID = command.Begin.TurnID
					state.turnCPUStart = current
					state.turnWallStart = time.Now()
					state.moveLimit = command.Begin.MoveCPULimit
					state.totalLimit = command.Begin.TotalCPULimit
					state.wallLimit = command.Begin.WallLimit
					replyErr = freezable.Resume()
					if replyErr != nil {
						state.active = false
					} else {
						turnTicker.Reset(defaultTurnTickInterval)
						turnTick = turnTicker.C
					}
				}
			case turnComplete, turnOutputExceeded:
				if !state.active || command.TurnID != state.turnID {
					replyErr = fmt.Errorf("turn %d is not active", command.TurnID)
				} else {
					typ := TurnCompleted
					if command.Type == turnOutputExceeded {
						typ = TurnOutputLimitExceeded
					}
					event, err := freezeAndCheck(time.Now(), typ)
					if err != nil {
						event = usageEvent(time.Now(), ControlError)
						event.Error = err.Error()
						replyErr = err
						command.Reply <- replyErr
						return fatal(event)
					}
					event.Output = command.Output
					state.active = false
					turnTicker.Stop()
					turnTick = nil
					if event.Type != TurnCompleted {
						command.Reply <- nil
						return fatal(event)
					}
					emit(event)
					state.turnID = 0
				}
			default:
				replyErr = fmt.Errorf("unknown turn command")
			}
			command.Reply <- replyErr
		case now := <-turnTick:
			if !state.active {
				continue
			}
			if event, exceeded := checkLimits(now); exceeded {
				finalEvent, err := freezeAndCheck(time.Now(), event.Type)
				if err != nil {
					event.Type = ControlError
					event.Error = err.Error()
				} else {
					event = finalEvent
				}
				return fatal(event)
			}
		case <-ordinaryTicker.C:
			clockLimit := w.clockTimeLimit
			if clockLimit == 0 {
				clockLimit = w.timeLimit
			}
			if time.Since(ordinaryStart) > clockLimit || process.Usage().Time > w.timeLimit {
				return true
			}
		}
	}
}
