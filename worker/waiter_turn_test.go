package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/criyle/go-judge/envexec"
)

type fakeFreezableProcess struct {
	mu      sync.Mutex
	usage   time.Duration
	frozen  bool
	done    chan struct{}
	freezeN int
	resumeN int
}

func newFakeFreezableProcess() *fakeFreezableProcess {
	return &fakeFreezableProcess{done: make(chan struct{})}
}

func (p *fakeFreezableProcess) Done() <-chan struct{} { return p.done }
func (p *fakeFreezableProcess) Result() envexec.RunnerResult {
	<-p.done
	return envexec.RunnerResult{}
}
func (p *fakeFreezableProcess) Usage() envexec.Usage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return envexec.Usage{Time: p.usage}
}
func (p *fakeFreezableProcess) Freeze() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.frozen = true
	p.freezeN++
	return nil
}
func (p *fakeFreezableProcess) Resume() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.frozen = false
	p.resumeN++
	return nil
}
func (p *fakeFreezableProcess) setUsage(usage time.Duration) {
	p.mu.Lock()
	p.usage = usage
	p.mu.Unlock()
}

func startControlledWaiter(t *testing.T, process *fakeFreezableProcess) (*RuntimeControl, context.CancelFunc, <-chan bool) {
	t.Helper()
	control := NewRuntimeControl(2)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() {
		done <- (&waiter{timeLimit: time.Hour, clockTimeLimit: time.Hour, control: control}).Wait(ctx, process)
	}()
	return control, cancel, done
}

func validBegin(turnID uint64) BeginTurn {
	return BeginTurn{
		TurnID: turnID, MoveCPULimit: 100 * time.Millisecond,
		TotalCPULimit: time.Second, WallLimit: time.Second,
		Delimiter: []byte("\n"), MaxOutput: 4096,
	}
}

func TestControlledWaiterCompletesTurnWithFinalUsage(t *testing.T) {
	process := newFakeFreezableProcess()
	control, cancel, done := startControlledWaiter(t, process)
	defer func() { cancel(); <-done }()

	if err := control.Freeze(context.Background()); err != nil {
		t.Fatal(err)
	}
	process.setUsage(10 * time.Millisecond)
	if err := control.BeginTurn(context.Background(), validBegin(17)); err != nil {
		t.Fatal(err)
	}
	process.setUsage(30 * time.Millisecond)
	if err := control.CompleteTurn(context.Background(), 17, []byte("MOVE\n")); err != nil {
		t.Fatal(err)
	}

	event := <-control.Events
	if event.Type != TurnCompleted || event.TurnID != 17 || event.Index != 2 {
		t.Fatalf("unexpected event: %#v", event)
	}
	if event.MoveCPU != 20*time.Millisecond || event.TotalCPU != 30*time.Millisecond {
		t.Fatalf("unexpected usage: %#v", event)
	}
	if string(event.Output) != "MOVE\n" || !process.frozen {
		t.Fatalf("turn was not frozen and captured: %#v", event)
	}
}

func TestControlledWaiterTotalCPUAcrossTurns(t *testing.T) {
	process := newFakeFreezableProcess()
	control, cancel, done := startControlledWaiter(t, process)
	defer cancel()

	if err := control.Freeze(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := validBegin(1)
	first.TotalCPULimit = 150 * time.Millisecond
	if err := control.BeginTurn(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	process.setUsage(80 * time.Millisecond)
	if err := control.CompleteTurn(context.Background(), 1, []byte("A\n")); err != nil {
		t.Fatal(err)
	}
	if event := <-control.Events; event.Type != TurnCompleted {
		t.Fatalf("first turn: %#v", event)
	}

	second := validBegin(2)
	second.TotalCPULimit = 150 * time.Millisecond
	if err := control.BeginTurn(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	process.setUsage(170 * time.Millisecond)
	if err := control.CompleteTurn(context.Background(), 2, []byte("B\n")); err != nil {
		t.Fatal(err)
	}
	if event := <-control.Events; event.Type != TotalCPULimitExceeded || event.TotalCPU != 170*time.Millisecond {
		t.Fatalf("second turn: %#v", event)
	}
	if timedOut := <-done; !timedOut {
		t.Fatal("fatal total CPU event did not stop the waiter")
	}
}

func TestControlledWaiterWallLimit(t *testing.T) {
	process := newFakeFreezableProcess()
	control, cancel, done := startControlledWaiter(t, process)
	defer cancel()

	if err := control.Freeze(context.Background()); err != nil {
		t.Fatal(err)
	}
	begin := validBegin(9)
	begin.WallLimit = 5 * time.Millisecond
	if err := control.BeginTurn(context.Background(), begin); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-control.Events:
		if event.Type != MoveWallLimitExceeded {
			t.Fatalf("unexpected event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("wall timeout event was not emitted")
	}
	if timedOut := <-done; !timedOut {
		t.Fatal("wall timeout did not stop the waiter")
	}
}

func TestControlledWaiterMoveCPULimit(t *testing.T) {
	process := newFakeFreezableProcess()
	control, cancel, done := startControlledWaiter(t, process)
	defer cancel()

	if err := control.Freeze(context.Background()); err != nil {
		t.Fatal(err)
	}
	begin := validBegin(3)
	begin.MoveCPULimit = minimumMoveCPULimit
	if err := control.BeginTurn(context.Background(), begin); err != nil {
		t.Fatal(err)
	}
	process.setUsage(60 * time.Millisecond)
	select {
	case event := <-control.Events:
		if event.Type != MoveCPULimitExceeded || event.MoveCPU != 60*time.Millisecond {
			t.Fatalf("unexpected event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("move CPU timeout event was not emitted")
	}
	if timedOut := <-done; !timedOut {
		t.Fatal("move CPU timeout did not stop the waiter")
	}
}

func TestBeginTurnRejectsSmallMoveLimit(t *testing.T) {
	control := NewRuntimeControl(0)
	begin := validBegin(1)
	begin.MoveCPULimit = minimumMoveCPULimit - time.Nanosecond
	if err := control.BeginTurn(context.Background(), begin); err == nil {
		t.Fatal("expected minimum move CPU limit error")
	}
}

type fakeProcess struct{ done chan struct{} }

func (p *fakeProcess) Done() <-chan struct{}        { return p.done }
func (p *fakeProcess) Result() envexec.RunnerResult { <-p.done; return envexec.RunnerResult{} }
func (p *fakeProcess) Usage() envexec.Usage         { return envexec.Usage{} }

func TestControlledWaiterRejectsNonFreezableProcess(t *testing.T) {
	process := &fakeProcess{done: make(chan struct{})}
	control := NewRuntimeControl(0)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() {
		done <- (&waiter{timeLimit: time.Hour, clockTimeLimit: time.Hour, control: control}).Wait(ctx, process)
	}()

	if err := control.BeginTurn(context.Background(), validBegin(1)); err == nil {
		t.Fatal("expected unsupported error")
	}
	cancel()
	if timedOut := <-done; timedOut {
		t.Fatal("unsupported command must not be reported as an ordinary timeout")
	}
}
