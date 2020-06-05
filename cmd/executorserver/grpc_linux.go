package main

import (
	"fmt"
	"io"

	"github.com/creack/pty"
	"github.com/criyle/go-judge/pb"
)

func (e *execServer) ExecStream(es pb.Executor_ExecStreamServer) error {
	msg, err := es.Recv()
	if err != nil {
		return err
	}
	req := msg.GetExecRequest()
	if req == nil {
		return fmt.Errorf("The first stream request must be exec request")
	}
	rq, streamIn, streamOut, err := convertPBRequest(req)
	if err != nil {
		return err
	}
	defer func() {
		for _, fi := range streamIn {
			fi.Close()
		}
		for _, fi := range streamOut {
			fi.Close()
		}
	}()

	errCh := make(chan error, 1)

	// stream in
	if len(streamIn) > 0 {
		go func() {
			err := streamInput(es, streamIn)
			if err != nil {
				writeErrCh(errCh, err)
			}
		}()
	}

	// stream out
	outCh := make(chan *pb.StreamResponse_ExecOutput, len(streamOut))
	if len(streamOut) > 0 {
		for _, so := range streamOut {
			so := so
			go func() {
				err := streamOutput(es.Context().Done(), outCh, so)
				if err != nil {
					writeErrCh(errCh, err)
				}
			}()
		}
	}

	rtCh := work.Execute(es.Context(), rq)
	for {
		select {
		case err := <-errCh:
			return err

		case o := <-outCh:
			err = es.Send(&pb.StreamResponse{
				Response: o,
			})
			if err != nil {
				return err
			}
			buffPool.Put(o.ExecOutput.Content[:cap(o.ExecOutput.Content)])

		case rt := <-rtCh:
			execObserve(rt)
			if rt.Error != nil {
				return err
			}
			return es.Send(&pb.StreamResponse{
				Response: &pb.StreamResponse_ExecResponse{
					ExecResponse: convertPBResponse(rt),
				},
			})
		}
	}
}

func streamOutput(done <-chan struct{}, outCh chan<- *pb.StreamResponse_ExecOutput, so *fileStreamOut) error {
	for {
		select {
		case <-done:
			return nil
		default:
		}

		buf := buffPool.Get().([]byte)
		n, err := so.Read(buf)
		if err != nil {
			return nil
		}
		outCh <- &pb.StreamResponse_ExecOutput{
			ExecOutput: &pb.StreamResponse_Output{
				Name:    so.Name(),
				Content: buf[:n],
			},
		}
	}
}

func streamInput(es pb.Executor_ExecStreamServer, streamIn []*fileStreamIn) error {
	inf := make(map[string]*fileStreamIn)
	for _, f := range streamIn {
		inf[f.Name()] = f
	}
	for {
		select {
		case <-es.Context().Done():
			return nil
		default:
		}

		in, err := es.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch i := in.Request.(type) {
		case *pb.StreamRequest_ExecInput:
			f, ok := inf[i.ExecInput.GetName()]
			if !ok {
				return fmt.Errorf("input %s not exists", i.ExecInput.GetName())
			}
			_, err = f.Write(i.ExecInput.Content)
			if err != nil {
				return fmt.Errorf("write to input %s with err %w", i.ExecInput.GetName(), err)
			}

		case *pb.StreamRequest_ExecResize:
			f, ok := inf[i.ExecResize.GetName()]
			if !ok {
				return fmt.Errorf("input %s not exists", i.ExecResize.GetName())
			}
			winSize := &pty.Winsize{
				Rows: uint16(i.ExecResize.Rows),
				Cols: uint16(i.ExecResize.Cols),
				X:    uint16(i.ExecResize.X),
				Y:    uint16(i.ExecResize.Y),
			}
			err = pty.Setsize(f.w, winSize)
			if err != nil {
				return fmt.Errorf("resize to input %s with err %w", i.ExecResize.GetName(), err)
			}

		default:
			return fmt.Errorf("the following request must be input request")
		}
	}
}

func writeErrCh(ch chan error, err error) {
	select {
	case ch <- err:
	default:
	}
}
