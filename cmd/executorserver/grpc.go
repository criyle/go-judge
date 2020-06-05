package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/creack/pty"
	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/pb"
	"github.com/criyle/go-judge/worker"
)

const buffLen = 4096

var buffPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, buffLen)
	},
}

type execServer struct {
	pb.UnimplementedExecutorServer
	fs filestore.FileStore
}

func (e *execServer) Exec(ctx context.Context, req *pb.Request) (*pb.Response, error) {
	r, si, so, err := convertPBRequest(req)
	if err != nil {
		return nil, err
	}
	if len(si) > 0 || len(so) > 0 {
		return nil, fmt.Errorf("Stream in / out are not avaliable for exec request")
	}
	rt := <-work.Submit(ctx, r)
	execObserve(rt)
	if rt.Error != nil {
		return nil, err
	}
	return convertPBResponse(rt), nil
}

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

func (e *execServer) FileList(c context.Context, n *pb.Empty) (*pb.FileListType, error) {
	return &pb.FileListType{
		FileIDs: e.fs.List(),
	}, nil
}

func (e *execServer) FileGet(c context.Context, f *pb.FileID) (*pb.FileContent, error) {
	file := e.fs.Get(f.GetFileID())
	content, err := file.Content()
	if err != nil {
		return nil, err
	}
	return &pb.FileContent{
		Name:    file.Name(),
		Content: content,
	}, nil
}

func (e *execServer) FileAdd(c context.Context, fc *pb.FileContent) (*pb.FileID, error) {
	fid, err := e.fs.Add(fc.GetName(), fc.GetContent())
	if err != nil {
		return nil, err
	}
	return &pb.FileID{
		FileID: fid,
	}, nil
}

func (e *execServer) FileDelete(c context.Context, f *pb.FileID) (*pb.Empty, error) {
	ok := e.fs.Remove(f.GetFileID())
	if !ok {
		return nil, fmt.Errorf("file id does not exists for %v", f.GetFileID())
	}
	return &pb.Empty{}, nil
}

func convertPBResponse(r worker.Response) *pb.Response {
	res := &pb.Response{
		RequestID: r.RequestID,
		Results:   make([]*pb.Response_Result, 0, len(r.Results)),
	}
	for _, c := range r.Results {
		res.Results = append(res.Results, convertPBResult(c))
	}
	if r.Error != nil {
		res.Error = r.Error.Error()
	}
	return res
}

func convertPBResult(r worker.Result) *pb.Response_Result {
	return &pb.Response_Result{
		Status:     pb.Response_Result_StatusType(r.Status),
		ExitStatus: int32(r.ExitStatus),
		Error:      r.Error,
		Time:       r.Time,
		RunTime:    r.RunTime,
		Memory:     r.Memory,
		Files:      r.Files,
		FileIDs:    r.FileIDs,
	}
}

func convertPBRequest(r *pb.Request) (req *worker.Request, streamIn []*fileStreamIn, streamOut []*fileStreamOut, err error) {
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
	req = &worker.Request{
		RequestID:   r.RequestID,
		Cmd:         make([]worker.Cmd, 0, len(r.Cmd)),
		PipeMapping: make([]worker.PipeMap, 0, len(r.PipeMapping)),
	}
	for _, c := range r.Cmd {
		cm, si, so, err := convertPBCmd(c)
		streamIn = append(streamIn, si...)
		streamOut = append(streamOut, so...)
		if err != nil {
			return nil, streamIn, streamOut, err
		}
		req.Cmd = append(req.Cmd, cm)
	}
	for _, p := range r.PipeMapping {
		pm := convertPBPipeMap(p)
		req.PipeMapping = append(req.PipeMapping, pm)
	}
	return req, streamIn, streamOut, nil
}

func convertPBPipeMap(p *pb.Request_PipeMap) worker.PipeMap {
	return worker.PipeMap{
		In: worker.PipeIndex{
			Index: int(p.GetIn().GetIndex()),
			Fd:    int(p.GetIn().GetFd()),
		},
		Out: worker.PipeIndex{
			Index: int(p.GetOut().GetIndex()),
			Fd:    int(p.GetOut().GetFd()),
		},
	}
}

func convertPBCmd(c *pb.Request_CmdType) (cm worker.Cmd, streamIn []*fileStreamIn, streamOut []*fileStreamOut, err error) {
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
	cm = worker.Cmd{
		Args:          c.GetArgs(),
		Env:           c.GetEnv(),
		CPULimit:      c.GetCPULimit(),
		RealCPULimit:  c.GetRealCPULimit(),
		MemoryLimit:   c.GetMemoryLimit(),
		ProcLimit:     c.GetProcLimit(),
		CopyOut:       c.GetCopyOut(),
		CopyOutCached: c.GetCopyOutCached(),
		CopyOutDir:    c.GetCopyOutDir(),
	}
	var (
		fPty, fTty *os.File
		ttyOut     *fileStreamOut
	)
	for _, f := range c.GetFiles() {
		var cf worker.CmdFile
		switch fi := f.File.(type) {
		case *pb.Request_File_StreamIn:
			var si *fileStreamIn
			if fi.StreamIn.Tty {
				fPty, fTty, err = pty.Open()
				if err != nil {
					return cm, streamIn, streamOut, err
				}
				si = &fileStreamIn{
					name: fi.StreamIn.GetName(),
					w:    fPty,
					r:    fTty,
				}
				streamIn = append(streamIn, si)
			} else {
				si, err = newFileStreamIn(fi.StreamIn.GetName())
				if err == nil {
					streamIn = append(streamIn, si)
				}
			}
			cf = si

		case *pb.Request_File_StreamOut:
			var so *fileStreamOut
			if fPty != nil {
				if ttyOut == nil {
					ttyOut = &fileStreamOut{
						name: fi.StreamOut.GetName(),
						w:    fTty,
						r:    fPty,
					}
					streamOut = append(streamOut, ttyOut)
				}
				so = ttyOut
			} else {
				so, err = newFileStreamOut(fi.StreamOut.GetName())
				if err == nil {
					streamOut = append(streamOut, so)
				}
			}
			cf = so

		default:
			cf, err = convertPBFile(f)
		}
		if err != nil {
			return cm, streamIn, streamOut, err
		}
		cm.Files = append(cm.Files, cf)
	}
	if copyIn := c.GetCopyIn(); copyIn != nil {
		cm.CopyIn = make(map[string]worker.CmdFile)
		for k, f := range copyIn {
			cf, err := convertPBFile(f)
			if err != nil {
				return cm, streamIn, streamOut, err
			}
			cm.CopyIn[k] = cf
		}
	}
	return cm, streamIn, streamOut, nil
}

func convertPBFile(c *pb.Request_File) (worker.CmdFile, error) {
	switch c := c.File.(type) {
	case nil:
		return nil, nil
	case *pb.Request_File_Local:
		return &worker.LocalFile{Src: c.Local.GetSrc()}, nil
	case *pb.Request_File_Memory:
		return &worker.MemoryFile{Content: c.Memory.GetContent()}, nil
	case *pb.Request_File_Cached:
		return &worker.CachedFile{FileID: c.Cached.GetFileID()}, nil
	case *pb.Request_File_Pipe:
		return &worker.PipeCollector{Name: c.Pipe.GetName(), Max: c.Pipe.GetMax()}, nil
	}
	return nil, fmt.Errorf("request file type not supported yet %v", c)
}
