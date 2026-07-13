package stream

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/criyle/go-judge/cmd/go-judge/model"
	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/worker"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const (
	newBuffLen = 32 << 10
	minBuffLen = 4 << 10
)

// Stream defines the transport layer for the stream execution that
// stream input and output interactively
type Stream interface {
	Send(Response) error
	Recv() (*Request, error)
}

// Request defines operations receive from the remote
type Request struct {
	Request *model.Request
	Resize  *ResizeRequest
	Input   *InputRequest
	Cancel  *struct{}
	Control *ControlRequest
}

// Response defines response to the remote
type Response struct {
	Response *model.Response
	Output   *OutputResponse
	Control  *ControlResponse
}

type ControlRequest struct {
	Index     int               `json:"index"`
	BeginTurn *BeginTurnRequest `json:"beginTurn,omitempty"`
}

type BeginTurnRequest struct {
	TurnID        uint64 `json:"turnId"`
	MoveCPULimit  uint64 `json:"moveCpuLimit"`
	TotalCPULimit uint64 `json:"totalCpuLimit"`
	WallLimit     uint64 `json:"wallLimit"`
	OutputFD      int    `json:"outputFd"`
	Delimiter     string `json:"delimiter"`
	MaxOutput     int    `json:"maxOutput"`
}

type ControlResponse struct {
	RequestID string               `json:"requestId,omitempty"`
	Index     int                  `json:"index"`
	TurnID    uint64               `json:"turnId"`
	Type      worker.TurnEventType `json:"type"`
	MoveCPU   uint64               `json:"moveCpu"`
	TotalCPU  uint64               `json:"totalCpu"`
	WallTime  uint64               `json:"wallTime"`
	Output    string               `json:"output,omitempty"`
	Error     string               `json:"error,omitempty"`
}

// ResizeRequest defines resize operation to the virtual terminal
type ResizeRequest struct {
	Index int `json:"index,omitempty"`
	Fd    int `json:"fd,omitempty"`
	Rows  int `json:"rows,omitempty"`
	Cols  int `json:"cols,omitempty"`
	X     int `json:"x,omitempty"`
	Y     int `json:"y,omitempty"`
}

// InputRequest defines input operation from the remote
type InputRequest struct {
	Index   int
	Fd      int
	Content []byte
}

// OutputResponse defines output result to the remote
type OutputResponse struct {
	Index   int
	Fd      int
	Content []byte
}

var (
	errFirstMustBeExec = errors.New("the first stream request must be exec request")
)

// Start initiate a interactive execution on the worker and transmit the request and response over Stream transport layer
func Start(baseCtx context.Context, s Stream, w worker.Worker, srcPrefix []string, logger *zap.Logger) error {
	req, err := s.Recv()
	if err != nil {
		return err
	}
	if req.Request == nil {
		return errFirstMustBeExec
	}
	rq, streamIn, streamOut, turnOutputs, err := convertStreamRequest(req.Request, srcPrefix)
	if err != nil {
		return fmt.Errorf("convert exec request: %w", err)
	}
	closeFunc := func() {
		for _, f := range streamIn {
			f.Close()
		}
		streamIn = nil
		for _, f := range streamOut {
			f.Close()
		}
		streamOut = nil
	}
	defer closeFunc()

	if ce := logger.Check(zap.DebugLevel, "request"); ce != nil {
		ce.Write(zap.String("body", fmt.Sprintf("%+v", rq)))
	}

	var wg errgroup.Group
	var outWG errgroup.Group
	execCtx, execCancel := context.WithCancel(baseCtx)
	defer execCancel()

	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	// stream in
	wg.Go(func() error {
		if err := streamInput(ctx, s, streamIn, turnOutputs, rq.RuntimeControls, execCancel); err != nil {
			cancel()
			return err
		}
		return nil
	})

	// stream out
	outCh := make(chan *OutputResponse, len(streamOut))
	outDone := make(chan error, 1)
	if len(streamOut) > 0 {
		for _, so := range streamOut {
			so := so
			outWG.Go(func() error {
				return streamOutput(ctx, outCh, so)
			})
		}
		go func() {
			err := outWG.Wait()
			close(outCh)
			outDone <- err
			close(outDone)
		}()
	} else {
		close(outCh)
		outDone <- nil
		close(outDone)
	}

	rtCh := w.Execute(execCtx, rq)
	controlEvents := mergeControlEvents(ctx, rq.RuntimeControls)
	err = sendLoop(ctx, s, outCh, outDone, controlEvents, rtCh, rq.RequestID, logger)

	cancel()
	closeFunc()
	err2 := wg.Wait()
	return errors.Join(err, err2)
}

func sendLoop(ctx context.Context, s Stream, outCh <-chan *OutputResponse, outDone <-chan error, controlCh <-chan worker.TurnEvent, rtCh <-chan worker.Response, requestID string, logger *zap.Logger) error {
	var (
		outClosed    bool
		resultReady  bool
		resultSend   *model.Response
		streamOutErr error
		sentControl  = make(map[string]bool)
	)
	sendControl := func(event worker.TurnEvent) error {
		key := fmt.Sprintf("%d/%d/%s", event.Index, event.TurnID, event.Type)
		if sentControl[key] {
			return nil
		}
		if err := s.Send(Response{Control: convertControlEvent(requestID, event)}); err != nil {
			return fmt.Errorf("send control event: %w", err)
		}
		sentControl[key] = true
		return nil
	}

	for {
		if outClosed && resultReady {
			if streamOutErr != nil {
				return streamOutErr
			}
			return s.Send(Response{Response: resultSend})
		}

		select {
		case <-ctx.Done(): // error occur
			return ctx.Err()

		case o, ok := <-outCh:
			if !ok {
				outClosed = true
				outCh = nil
				streamOutErr = <-outDone
				outDone = nil
				continue
			}
			err := s.Send(Response{Output: o})
			if err != nil {
				return fmt.Errorf("send output: %w", err)
			}

		case event, ok := <-controlCh:
			if !ok {
				controlCh = nil
				continue
			}
			if err := sendControl(event); err != nil {
				return err
			}

		case rt := <-rtCh:
			if ce := logger.Check(zap.DebugLevel, "response"); ce != nil {
				ce.Write(zap.String("body", fmt.Sprintf("%+v", rt)))
			}
			for _, event := range rt.ControlEvents {
				if err := sendControl(event); err != nil {
					return err
				}
			}
			ret, err := model.ConvertResponse(rt, false)
			if err != nil {
				return fmt.Errorf("convert response: %w", err)
			}
			resultReady = true
			resultSend = &model.Response{Results: ret.Results}
			rtCh = nil
		}
	}
}

func convertControlEvent(requestID string, event worker.TurnEvent) *ControlResponse {
	return &ControlResponse{
		RequestID: requestID,
		Index:     event.Index,
		TurnID:    event.TurnID,
		Type:      event.Type,
		MoveCPU:   uint64(event.MoveCPU),
		TotalCPU:  uint64(event.TotalCPU),
		WallTime:  uint64(event.WallTime),
		Output:    string(event.Output),
		Error:     event.Error,
	}
}

func mergeControlEvents(ctx context.Context, controls []*worker.RuntimeControl) <-chan worker.TurnEvent {
	out := make(chan worker.TurnEvent, len(controls)*2)
	var wg sync.WaitGroup
	for _, control := range controls {
		if control == nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case event := <-control.Events:
					select {
					case <-ctx.Done():
						return
					case out <- event:
					}
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func convertStreamRequest(m *model.Request, srcPrefix []string) (req *worker.Request, streamIn []*fileStreamIn, streamOut []*fileStreamOut, turnOutputs map[int]*turnOutput, err error) {
	type cmdStream struct {
		index int
		fd    int
		f     worker.CmdFile
	}
	defer func() {
		if err != nil {
			for _, fi := range streamIn {
				fi.Close()
			}
			streamIn = nil
			for _, fi := range streamOut {
				fi.Close()
			}
			streamOut = nil
		}
	}()
	turnOutputs = make(map[int]*turnOutput)
	coordinator := new(turnCoordinator)
	var streams []cmdStream
	for i, c := range m.Cmd {
		for j, f := range c.Files {
			switch {
			case f == nil:
				continue
			case f.StreamIn:
				si := newFileStreamIn(i, j, c.TTY)
				streamIn = append(streamIn, si)
				streams = append(streams, cmdStream{index: i, fd: j, f: si})
				c.Files[j] = nil
			case f.StreamOut:
				so := newFileStreamOut(i, j)
				streamOut = append(streamOut, so)
				streams = append(streams, cmdStream{index: i, fd: j, f: so})
				c.Files[j] = nil
			}
		}
	}
	req, err = model.ConvertRequest(m, srcPrefix)
	if err != nil {
		return req, streamIn, streamOut, turnOutputs, err
	}
	req.RuntimeControls = make([]*worker.RuntimeControl, len(req.Cmd))
	for i := range req.Cmd {
		req.RuntimeControls[i] = worker.NewRuntimeControl(i)
	}
	for _, f := range streams {
		req.Cmd[f.index].Files[f.fd] = f.f
		if so, ok := f.f.(*fileStreamOut); ok {
			output := &turnOutput{stream: so, control: req.RuntimeControls[f.index], coordinator: coordinator}
			so.turn = output
			turnOutputs[f.index<<8|f.fd] = output
		}
	}
	return
}

func streamInput(ctx context.Context, s Stream, si []*fileStreamIn, outputs map[int]*turnOutput, controls []*worker.RuntimeControl, execCancel func()) error {
	inf := make(map[int]*fileStreamIn)
	for _, f := range si {
		inf[f.index<<8|f.fd] = f
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		in, err := s.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		switch {
		case in.Input != nil:
			f, ok := inf[in.Input.Index<<8|in.Input.Fd]
			if !ok {
				return fmt.Errorf("input does not exist: %d/%d", in.Input.Index, in.Input.Fd)
			}
			_, err := f.Write(in.Input.Content)
			if err == io.EOF { // file closed with io.EOF
				return nil
			}
			if err != nil {
				return fmt.Errorf("write to input %d/%d: %w", in.Input.Index, in.Input.Fd, err)
			}

		case in.Resize != nil:
			f, ok := inf[in.Resize.Index<<8|in.Resize.Fd]
			if !ok {
				return fmt.Errorf("input does not exist: %d/%d", in.Resize.Index, in.Resize.Fd)
			}
			if err = f.SetSize(&envexec.TerminalSize{
				Cols: uint16(in.Resize.Cols),
				Rows: uint16(in.Resize.Rows),
				X:    uint16(in.Resize.X),
				Y:    uint16(in.Resize.Y),
			}); err != nil {
				return fmt.Errorf("resize %d/%d: %w", in.Resize.Index, in.Resize.Fd, err)
			}

		case in.Cancel != nil:
			execCancel()
			return nil

		case in.Control != nil:
			if err := beginTurn(ctx, in.Control, outputs, controls); err != nil {
				turnID := uint64(0)
				if in.Control.BeginTurn != nil {
					turnID = in.Control.BeginTurn.TurnID
				}
				if in.Control.Index >= 0 && in.Control.Index < len(controls) && controls[in.Control.Index] != nil {
					controls[in.Control.Index].ControlError(turnID, err)
				} else if len(controls) > 0 && controls[0] != nil {
					controls[0].ControlErrorAt(in.Control.Index, turnID, err)
				}
				execCancel()
				return nil
			}

		default:
			return fmt.Errorf("invalid request")
		}
	}
}

func beginTurn(ctx context.Context, request *ControlRequest, outputs map[int]*turnOutput, controls []*worker.RuntimeControl) error {
	if request.BeginTurn == nil {
		return fmt.Errorf("control request has no operation")
	}
	if request.Index < 0 || request.Index >= len(controls) || controls[request.Index] == nil {
		return fmt.Errorf("control command does not exist: %d", request.Index)
	}
	b := request.BeginTurn
	output := outputs[request.Index<<8|b.OutputFD]
	if output == nil {
		return fmt.Errorf("controlled output does not exist: %d/%d", request.Index, b.OutputFD)
	}
	if err := output.coordinator.begin(request.Index, b.TurnID); err != nil {
		return err
	}
	started := false
	defer func() {
		if !started {
			output.coordinator.finish(request.Index, b.TurnID)
		}
	}()

	// The first controlled turn establishes the invariant that every other AI is frozen.
	for i, control := range controls {
		if control == nil {
			continue
		}
		if err := control.Freeze(ctx); err != nil {
			return fmt.Errorf("freeze command %d: %w", i, err)
		}
	}
	begin := worker.BeginTurn{
		TurnID:        b.TurnID,
		MoveCPULimit:  time.Duration(b.MoveCPULimit),
		TotalCPULimit: time.Duration(b.TotalCPULimit),
		WallLimit:     time.Duration(b.WallLimit),
		OutputFD:      b.OutputFD,
		Delimiter:     []byte(b.Delimiter),
		MaxOutput:     b.MaxOutput,
	}
	if err := output.beginTurn(ctx, begin); err != nil {
		return fmt.Errorf("begin turn: %w", err)
	}
	started = true
	return nil
}

type turnOutput struct {
	stream      *fileStreamOut
	control     *worker.RuntimeControl
	mu          sync.Mutex
	active      bool
	current     worker.BeginTurn
	buf         []byte
	coordinator *turnCoordinator
}

type turnCoordinator struct {
	mu     sync.Mutex
	active bool
	index  int
	turnID uint64
}

func (c *turnCoordinator) begin(index int, turnID uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active {
		return fmt.Errorf("turn %d for command %d is still active", c.turnID, c.index)
	}
	c.active = true
	c.index = index
	c.turnID = turnID
	return nil
}

func (c *turnCoordinator) finish(index int, turnID uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active && c.index == index && c.turnID == turnID {
		c.active = false
	}
}

func (t *turnOutput) beginTurn(ctx context.Context, begin worker.BeginTurn) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.active {
		return fmt.Errorf("turn %d output is still active", t.current.TurnID)
	}
	t.active = true
	t.current = begin
	t.buf = t.buf[:0]
	if err := t.control.BeginTurn(ctx, begin); err != nil {
		t.active = false
		t.buf = t.buf[:0]
		return err
	}
	return nil
}

func (t *turnOutput) consume(ctx context.Context, content []byte) (bool, error) {
	t.mu.Lock()
	if !t.active {
		t.mu.Unlock()
		return false, nil
	}
	t.buf = append(t.buf, content...)
	begin := t.current
	boundary := bytes.Index(t.buf, begin.Delimiter)
	if boundary >= 0 {
		boundary += len(begin.Delimiter)
	}
	if boundary < 0 && len(t.buf) <= begin.MaxOutput {
		t.mu.Unlock()
		return true, nil
	}
	if boundary > begin.MaxOutput || (boundary < 0 && len(t.buf) > begin.MaxOutput) {
		t.active = false
		t.buf = t.buf[:0]
		t.mu.Unlock()
		err := t.control.OutputExceeded(ctx, begin.TurnID)
		t.coordinator.finish(t.controlIndex(), begin.TurnID)
		return true, err
	}
	output := append([]byte(nil), t.buf[:boundary]...)
	t.active = false
	t.buf = t.buf[:0]
	t.mu.Unlock()
	err := t.control.CompleteTurn(ctx, begin.TurnID, output)
	t.coordinator.finish(t.controlIndex(), begin.TurnID)
	return true, err
}

func (t *turnOutput) controlIndex() int {
	return t.control.Index()
}

func streamOutput(ctx context.Context, outCh chan *OutputResponse, so *fileStreamOut) error {
	var buf []byte
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if len(buf) < minBuffLen {
			buf = make([]byte, newBuffLen)
		}

		n, err := so.Read(buf)
		if err != nil { // file closed with io.EOF
			return nil
		}
		content := buf[:n]
		if so.turn != nil {
			consumed, err := so.turn.consume(ctx, content)
			if err != nil {
				return err
			}
			if consumed {
				buf = buf[n:]
				continue
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case outCh <- &OutputResponse{
			Index:   so.index,
			Fd:      so.fd,
			Content: content,
		}:
		}
		buf = buf[n:]
	}
}
