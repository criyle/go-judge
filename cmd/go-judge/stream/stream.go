package stream

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/criyle/go-judge/cmd/go-judge/model"
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
}

// Response defines response to the remote
type Response struct {
	Response *model.Response
	Output   *OutputResponse
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
	rq, streamIn, streamOut, err := convertStreamRequest(req.Request, srcPrefix)
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
	execCtx, execCancel := context.WithCancel(baseCtx)
	defer execCancel()

	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	// stream in
	wg.Go(func() error {
		if err := streamInput(ctx, s, streamIn, execCancel); err != nil {
			cancel()
			return err
		}
		return nil
	})

	// stream out
	outCh := make(chan *OutputResponse, len(streamOut))
	if len(streamOut) > 0 {
		for _, so := range streamOut {
			so := so
			wg.Go(func() error {
				return streamOutput(ctx, outCh, so)
			})
		}
	}

	rtCh := w.Execute(execCtx, rq)
	err = sendLoop(ctx, s, outCh, rtCh, logger)

	cancel()
	closeFunc()
	streamOut = nil
	wg.Wait()
	return err
}

func sendLoop(ctx context.Context, s Stream, outCh chan *OutputResponse, rtCh <-chan worker.Response, logger *zap.Logger) error {
	for {
		select {
		case <-ctx.Done(): // error occur
			return ctx.Err()

		case o := <-outCh:
			err := s.Send(Response{Output: o})
			if err != nil {
				return fmt.Errorf("send output: %w", err)
			}

		case rt := <-rtCh:
			if ce := logger.Check(zap.DebugLevel, "response"); ce != nil {
				ce.Write(zap.String("body", fmt.Sprintf("%+v", rt)))
			}
			ret, err := model.ConvertResponse(rt, false)
			if err != nil {
				return fmt.Errorf("convert response: %w", err)
			}
			return s.Send(Response{Response: &model.Response{Results: ret.Results}})
		}
	}
}

func convertStreamRequest(m *model.Request, srcPrefix []string) (req *worker.Request, streamIn []*fileStreamIn, streamOut []*fileStreamOut, err error) {
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
		return req, streamIn, streamOut, err
	}
	for _, f := range streams {
		req.Cmd[f.index].Files[f.fd] = f.f
	}
	return
}

func streamInput(ctx context.Context, s Stream, si []*fileStreamIn, execCancel func()) error {
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
			tty := f.GetTTY()
			if tty == nil {
				return fmt.Errorf("resize input does not have tty: %d/%d", in.Resize.Index, in.Resize.Fd)
			}
			if err = setWinsize(tty, in.Resize); err != nil {
				return fmt.Errorf("resize %d/%d: %w", in.Resize.Index, in.Resize.Fd, err)
			}

		case in.Cancel != nil:
			execCancel()
			return nil

		default:
			return fmt.Errorf("invalid request")
		}
	}
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
		select {
		case <-ctx.Done():
			return nil
		case outCh <- &OutputResponse{
			Index:   so.index,
			Fd:      so.fd,
			Content: buf[:n],
		}:
		}
		buf = buf[n:]
	}
}
