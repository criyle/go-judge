package grpcexecutor

import (
	"errors"

	"github.com/criyle/go-judge/cmd/go-judge/model"
	"github.com/criyle/go-judge/cmd/go-judge/stream"
	"github.com/criyle/go-judge/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ stream.Stream = &streamWrapper{}

type streamWrapper struct {
	es pb.Executor_ExecStreamServer
}

func (sw *streamWrapper) Send(r stream.StreamResponse) error {
	res := &pb.StreamResponse{}
	switch {
	case r.Response != nil:
		resp, err := convertPBResponse(*r.Response)
		if err != nil {
			return status.Errorf(codes.Aborted, "response: %v", err)
		}
		res.Response = &pb.StreamResponse_ExecResponse{ExecResponse: resp}
	case r.Output != nil:
		res.Response = &pb.StreamResponse_ExecOutput{ExecOutput: &pb.StreamResponse_Output{
			Name:    r.Output.Name,
			Content: r.Output.Content,
		}}
	}
	return sw.es.Send(res)
}

func (sw *streamWrapper) Recv() (*stream.StreamRequest, error) {
	req, err := sw.es.Recv()
	if err != nil {
		return nil, err
	}
	switch i := req.Request.(type) {
	case *pb.StreamRequest_ExecRequest:
		return &stream.StreamRequest{Request: convertPBStreamRequest(i.ExecRequest)}, nil
	case *pb.StreamRequest_ExecInput:
		return &stream.StreamRequest{Input: &stream.InputRequest{
			Name:    i.ExecInput.Name,
			Content: i.ExecInput.Content,
		}}, nil
	case *pb.StreamRequest_ExecResize:
		return &stream.StreamRequest{Resize: &stream.ResizeRequest{
			Name: i.ExecResize.Name,
			Rows: int(i.ExecResize.Rows),
			Cols: int(i.ExecResize.Cols),
			X:    int(i.ExecResize.X),
			Y:    int(i.ExecResize.Y),
		}}, nil
	case *pb.StreamRequest_ExecCancel:
		return &stream.StreamRequest{Cancel: &struct{}{}}, nil
	}
	return nil, errors.ErrUnsupported
}

func convertPBStreamRequest(req *pb.Request) *model.Request {
	ret := &model.Request{
		RequestID: req.RequestID,
	}
	for _, cmd := range req.Cmd {
		ret.Cmd = append(ret.Cmd, model.Cmd{
			Args:              cmd.Args,
			Env:               cmd.Env,
			TTY:               cmd.Tty,
			Files:             convertPBStreamFiles(cmd.Files),
			CPULimit:          cmd.CpuTimeLimit,
			ClockLimit:        cmd.ClockTimeLimit,
			MemoryLimit:       cmd.MemoryLimit,
			StackLimit:        cmd.StackLimit,
			ProcLimit:         cmd.ProcLimit,
			CPURateLimit:      cmd.CpuRateLimit,
			CPUSetLimit:       cmd.CpuSetLimit,
			DataSegmentLimit:  cmd.DataSegmentLimit,
			AddressSpaceLimit: cmd.AddressSpaceLimit,
			CopyIn:            convertPBStreamCopyIn(cmd),
			CopyOut:           convertStreamCopyOut(cmd.CopyOut),
			CopyOutCached:     convertStreamCopyOut(cmd.CopyOutCached),
			CopyOutMax:        cmd.CopyOutMax,
			CopyOutDir:        cmd.CopyOutDir,
		})
	}
	for _, p := range req.PipeMapping {
		ret.PipeMapping = append(ret.PipeMapping, model.PipeMap{
			In: model.PipeIndex{
				Index: int(p.In.Index),
				Fd:    int(p.In.Fd),
			},
			Out: model.PipeIndex{
				Index: int(p.Out.Index),
				Fd:    int(p.Out.Fd),
			},
			Max:   int64(p.Max),
			Name:  p.Name,
			Proxy: p.Proxy,
		})
	}
	return ret
}

func convertPBStreamFiles(files []*pb.Request_File) []*model.CmdFile {
	var rt []*model.CmdFile
	for _, f := range files {
		if f == nil {
			rt = append(rt, nil)
		} else {
			m := convertPBStreamFile(f)
			rt = append(rt, &m)
		}
	}
	return rt
}

func convertPBStreamCopyIn(cmd *pb.Request_CmdType) map[string]model.CmdFile {
	rt := make(map[string]model.CmdFile, len(cmd.CopyIn)+len(cmd.Symlinks))
	for k, i := range cmd.CopyIn {
		if i.File == nil {
			continue
		}
		rt[k] = convertPBStreamFile(i)
	}
	for k, v := range cmd.Symlinks {
		rt[k] = model.CmdFile{Symlink: &v}
	}
	return rt
}

func convertPBStreamFile(i *pb.Request_File) model.CmdFile {
	switch c := i.File.(type) {
	case *pb.Request_File_Local:
		return model.CmdFile{Src: &c.Local.Src}
	case *pb.Request_File_Memory:
		s := byteArrayToString(c.Memory.Content)
		return model.CmdFile{Content: &s}
	case *pb.Request_File_Cached:
		return model.CmdFile{FileID: &c.Cached.FileID}
	case *pb.Request_File_Pipe:
		return model.CmdFile{Name: &c.Pipe.Name, Max: &c.Pipe.Max, Pipe: c.Pipe.Pipe}
	case *pb.Request_File_StreamIn:
		return model.CmdFile{StreamIn: &c.StreamIn.Name}
	case *pb.Request_File_StreamOut:
		return model.CmdFile{StreamOut: &c.StreamOut.Name}
	}
	return model.CmdFile{}
}

func convertStreamCopyOut(copyOut []*pb.Request_CmdCopyOutFile) []string {
	rt := make([]string, 0, len(copyOut))
	for _, n := range copyOut {
		name := n.Name
		if n.Optional {
			name += "?"
		}
		rt = append(rt, name)
	}
	return rt
}

func (e *execServer) ExecStream(es pb.Executor_ExecStreamServer) error {
	w := &streamWrapper{
		es: es,
	}
	if err := stream.Start(es.Context(), w, e.worker, e.srcPrefix, e.logger); err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	return nil
}
