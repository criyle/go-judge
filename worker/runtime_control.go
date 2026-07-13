package worker

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	TurnCompleted           TurnEventType = "turnCompleted"
	MoveCPULimitExceeded    TurnEventType = "moveCpuLimitExceeded"
	TotalCPULimitExceeded   TurnEventType = "totalCpuLimitExceeded"
	MoveWallLimitExceeded   TurnEventType = "moveWallLimitExceeded"
	TurnOutputLimitExceeded TurnEventType = "turnOutputLimitExceeded"
	ProcessExited           TurnEventType = "processExited"
	ControlError            TurnEventType = "controlError"
	defaultTurnTickInterval               = time.Millisecond
	minimumMoveCPULimit                   = 50 * time.Millisecond
)

type TurnEventType string

type BeginTurn struct {
	TurnID        uint64
	MoveCPULimit  time.Duration
	TotalCPULimit time.Duration
	WallLimit     time.Duration
	OutputFD      int
	Delimiter     []byte
	MaxOutput     int
}

type TurnCommandType uint8

const (
	turnBegin TurnCommandType = iota
	turnComplete
	turnOutputExceeded
	turnFreeze
)

type TurnCommand struct {
	Type   TurnCommandType
	Begin  BeginTurn
	TurnID uint64
	Output []byte
	Reply  chan error
}

type TurnEvent struct {
	Index    int
	TurnID   uint64
	Type     TurnEventType
	MoveCPU  time.Duration
	TotalCPU time.Duration
	WallTime time.Duration
	Output   []byte
	Error    string
}

type RuntimeControl struct {
	Commands chan TurnCommand
	Events   chan TurnEvent
	index    int
	fatal    func()
	stopped  chan struct{}
	mu       sync.Mutex
	history  []TurnEvent
}

func (r *RuntimeControl) publish(event TurnEvent) {
	event.Index = r.index
	r.publishAt(event)
}

func (r *RuntimeControl) EventsSnapshot() []TurnEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]TurnEvent(nil), r.history...)
}

func (r *RuntimeControl) ControlError(turnID uint64, err error) {
	r.publish(TurnEvent{TurnID: turnID, Type: ControlError, Error: err.Error()})
}

func (r *RuntimeControl) ControlErrorAt(index int, turnID uint64, err error) {
	r.publishAt(TurnEvent{Index: index, TurnID: turnID, Type: ControlError, Error: err.Error()})
}

func (r *RuntimeControl) publishAt(event TurnEvent) {
	r.mu.Lock()
	r.history = append(r.history, event)
	r.mu.Unlock()
	select {
	case r.Events <- event:
	default:
	}
}

func NewRuntimeControl(index int) *RuntimeControl {
	return &RuntimeControl{
		Commands: make(chan TurnCommand, 8),
		Events:   make(chan TurnEvent, 8),
		index:    index,
		stopped:  make(chan struct{}),
	}
}

func (r *RuntimeControl) Index() int { return r.index }

func (r *RuntimeControl) send(ctx context.Context, command TurnCommand) error {
	command.Reply = make(chan error, 1)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.stopped:
		return fmt.Errorf("controlled process is not running")
	case r.Commands <- command:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.stopped:
		select {
		case err := <-command.Reply:
			return err
		default:
			return fmt.Errorf("controlled process is not running")
		}
	case err := <-command.Reply:
		return err
	}
}

func (r *RuntimeControl) Freeze(ctx context.Context) error {
	return r.send(ctx, TurnCommand{Type: turnFreeze})
}

func (r *RuntimeControl) BeginTurn(ctx context.Context, begin BeginTurn) error {
	if begin.MoveCPULimit > 0 && begin.MoveCPULimit < minimumMoveCPULimit {
		return fmt.Errorf("move CPU limit must be at least %s", minimumMoveCPULimit)
	}
	if len(begin.Delimiter) == 0 {
		return fmt.Errorf("turn delimiter must not be empty")
	}
	if begin.MaxOutput <= 0 {
		return fmt.Errorf("turn max output must be positive")
	}
	return r.send(ctx, TurnCommand{Type: turnBegin, Begin: begin})
}

func (r *RuntimeControl) CompleteTurn(ctx context.Context, turnID uint64, output []byte) error {
	return r.send(ctx, TurnCommand{Type: turnComplete, TurnID: turnID, Output: output})
}

func (r *RuntimeControl) OutputExceeded(ctx context.Context, turnID uint64) error {
	return r.send(ctx, TurnCommand{Type: turnOutputExceeded, TurnID: turnID})
}
