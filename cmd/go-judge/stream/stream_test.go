package stream

import (
	"context"
	"sync"
	"testing"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/worker"
	"go.uber.org/zap"
)

type recordingStream struct {
	mu    sync.Mutex
	sends []Response
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
	errCh := make(chan error, 1)

	rtCh <- worker.Response{
		Results: []worker.Result{{
			Status: envexec.StatusAccepted,
		}},
	}

	go func() {
		errCh <- sendLoop(context.Background(), s, outCh, outDone, rtCh, zap.NewNop())
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
