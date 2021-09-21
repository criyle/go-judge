package grpcexecutor

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/criyle/go-judge/cmd/executorserver/model"
	"github.com/criyle/go-judge/pb"
	"github.com/criyle/go-judge/worker"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (e *execServer) ExecStream(es pb.Executor_ExecStreamServer) error {
	msg, err := es.Recv()
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	req := msg.GetExecRequest()
	if req == nil {
		return status.Error(codes.InvalidArgument, "the first stream request must be exec request")
	}
	rq, streamIn, streamOut, err := convertPBRequest(req, e.srcPrefix)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "convert exec request: %v", err)
	}
	e.logger.Sugar().Debugf("request: %+v", rq)
	defer func() {
		for _, fi := range streamIn {
			fi.Close()
		}
		for _, fi := range streamOut {
			fi.Close()
		}
	}()

	ctx, cancel := context.WithCancel(es.Context())
	defer cancel()
	errCh := make(chan error, 1)
	var wg sync.WaitGroup

	// stream in
	if len(streamIn) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := streamInput(ctx, es, streamIn); err != nil {
				writeErrCh(errCh, err)
			}
		}()
	}

	// stream out
	outCh := make(chan *pb.StreamResponse_ExecOutput, len(streamOut))
	if len(streamOut) > 0 {
		wg.Add(len(streamOut))
		for _, so := range streamOut {
			so := so
			go func() {
				defer wg.Done()
				if err := streamOutput(ctx, outCh, so); err != nil {
					writeErrCh(errCh, err)
				}
			}()
		}
	}

	rtCh := e.worker.Execute(es.Context(), rq)
	err = execStreamLoop(es, errCh, outCh, rtCh, e.logger)

	// Ensure all goroutine are exited
	cancel()
	wg.Wait()
	return err
}

func execStreamLoop(es pb.Executor_ExecStreamServer, errCh chan error, outCh chan *pb.StreamResponse_ExecOutput, rtCh <-chan worker.Response, logger *zap.Logger) error {
	for {
		select {
		case <-es.Context().Done():
			if err := es.Context().Err(); err != nil {
				return status.Errorf(codes.Canceled, "context finished: %v", err)
			}
			return nil

		case err := <-errCh:
			return status.Errorf(codes.Aborted, "stream in/out: %v", err)

		case o := <-outCh:
			err := es.Send(&pb.StreamResponse{
				Response: o,
			})
			if err != nil {
				return status.Errorf(codes.Aborted, "output: %v", err)
			}
			buffPool.Put(o.ExecOutput.Content[:cap(o.ExecOutput.Content)])

		case rt := <-rtCh:
			logger.Sugar().Debugf("response: %+v", rt)
			ret, err := model.ConvertResponse(rt, false)
			if err != nil {
				return status.Errorf(codes.Aborted, "response: %v", err)
			}

			resp, err := convertPBResponse(ret)
			if err != nil {
				return status.Errorf(codes.Aborted, "response: %v", err)
			}
			return es.Send(&pb.StreamResponse{
				Response: &pb.StreamResponse_ExecResponse{
					ExecResponse: resp,
				},
			})
		}
	}
}

func streamOutput(ctx context.Context, outCh chan<- *pb.StreamResponse_ExecOutput, so *fileStreamOut) error {
	for {
		select {
		case <-ctx.Done():
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

func streamInput(ctx context.Context, es pb.Executor_ExecStreamServer, streamIn []*fileStreamIn) error {
	inf := make(map[string]*fileStreamIn)
	for _, f := range streamIn {
		inf[f.Name()] = f
	}
	for {
		select {
		case <-ctx.Done():
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
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("write to input %s with err %w", i.ExecInput.GetName(), err)
			}

		case *pb.StreamRequest_ExecResize:
			f, ok := inf[i.ExecResize.GetName()]
			if !ok {
				return fmt.Errorf("input %s not exists", i.ExecResize.GetName())
			}
			tty := f.GetTTY()
			if tty == nil {
				return fmt.Errorf("input %s does not have TTY", i.ExecResize.GetName())
			}
			if err = setWinsize(tty, i); err != nil {
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
