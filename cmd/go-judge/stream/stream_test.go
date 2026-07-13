package stream

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/worker"
	"go.uber.org/zap"
)

type recordingStream struct {
	mu    sync.Mutex
	sends []Response
}

func TestTurnOutputDelimiterAcrossChunks(t *testing.T) {
	control := worker.NewRuntimeControl(0)
	commands := make(chan worker.TurnCommand, 1)

	output := &turnOutput{control: control, coordinator: new(turnCoordinator)}
	begin := worker.BeginTurn{
		TurnID: 1, MoveCPULimit: 100 * time.Millisecond,
		TotalCPULimit: time.Second, WallLimit: time.Second,
		Delimiter: []byte("\r\n"), MaxOutput: 32,
	}
	beginCommands := make(chan worker.TurnCommand, 1)
	go func() {
		command := <-control.Commands
		command.Reply <- nil
		beginCommands <- command
		command = <-control.Commands
		commands <- command
		command.Reply <- nil
	}()
	if err := output.beginTurn(context.Background(), begin); err != nil {
		t.Fatal(err)
	}
	<-beginCommands
	if consumed, err := output.consume(context.Background(), []byte("MOVE\r")); err != nil || !consumed {
		t.Fatalf("first chunk: consumed=%v err=%v", consumed, err)
	}
	if consumed, err := output.consume(context.Background(), []byte("\ntrailing")); err != nil || !consumed {
		t.Fatalf("second chunk: consumed=%v err=%v", consumed, err)
	}
	command := <-commands
	if command.TurnID != 1 || string(command.Output) != "MOVE\r\n" {
		t.Fatalf("unexpected command: %#v", command)
	}
}

func TestTurnOutputLimitExceeded(t *testing.T) {
	control := worker.NewRuntimeControl(0)
	commands := make(chan worker.TurnCommand, 1)

	output := &turnOutput{control: control, coordinator: new(turnCoordinator)}
	begin := worker.BeginTurn{
		TurnID: 2, MoveCPULimit: 100 * time.Millisecond,
		TotalCPULimit: time.Second, WallLimit: time.Second,
		Delimiter: []byte("\n"), MaxOutput: 4,
	}
	beginCommands := make(chan worker.TurnCommand, 1)
	go func() {
		command := <-control.Commands
		command.Reply <- nil
		beginCommands <- command
		command = <-control.Commands
		commands <- command
		command.Reply <- nil
	}()
	if err := output.beginTurn(context.Background(), begin); err != nil {
		t.Fatal(err)
	}
	<-beginCommands
	if _, err := output.consume(context.Background(), []byte("12345")); err != nil {
		t.Fatal(err)
	}
	command := <-commands
	if command.TurnID != 2 || command.Output != nil {
		t.Fatalf("unexpected command: %#v", command)
	}
}

func (r *recordingStream) Send(resp Response) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sends = append(r.sends, resp)
	return nil
}

func (*recordingStream) Recv() (*Request, error) {
	return nil, nil
}

func TestSendLoopDrainsOutputBeforeFinalResponse(t *testing.T) {
	s := &recordingStream{}
	outCh := make(chan *OutputResponse)
	outDone := make(chan error, 1)
	rtCh := make(chan worker.Response, 1)
	controlCh := make(chan worker.TurnEvent)
	errCh := make(chan error, 1)

	rtCh <- worker.Response{
		Results: []worker.Result{{
			Status: envexec.StatusAccepted,
		}},
	}

	go func() {
		errCh <- sendLoop(context.Background(), s, outCh, outDone, controlCh, rtCh, "", zap.NewNop())
	}()

	outCh <- &OutputResponse{Index: 0, Fd: 1, Content: []byte("tail")}
	close(outCh)
	outDone <- nil
	close(outDone)

	if err := <-errCh; err != nil {
		t.Fatalf("sendLoop returned error: %v", err)
	}

	if len(s.sends) != 2 {
		t.Fatalf("expected 2 sends, got %d", len(s.sends))
	}
	if s.sends[0].Output == nil || string(s.sends[0].Output.Content) != "tail" {
		t.Fatalf("expected output frame first, got %#v", s.sends[0])
	}
	if s.sends[1].Response == nil {
		t.Fatalf("expected final response second, got %#v", s.sends[1])
	}
}
