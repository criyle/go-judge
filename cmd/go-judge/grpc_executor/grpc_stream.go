package grpcexecutor

import (
	"errors"

	"github.com/criyle/go-judge/cmd/go-judge/model"
	"github.com/criyle/go-judge/cmd/go-judge/stream"
	"github.com/criyle/go-judge/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var _ stream.Stream = &streamWrapper{}

type streamWrapper struct {
	es pb.Executor_ExecStreamServer
}

func (sw *streamWrapper) Send(r stream.Response) error {
	res := &pb.StreamResponse{}
	switch {
	case r.Response != nil:
		resp, err := convertPBResponse(*r.Response)
		if err != nil {
			return status.Errorf(codes.Aborted, "response: %v", err)
		}
		res.SetExecResponse(proto.ValueOrDefault(resp))
	case r.Output != nil:
		res.SetExecOutput(pb.StreamResponse_Output_builder{
			Index:   uint32(r.Output.Index),
			Fd:      uint32(r.Output.Fd),
			Content: r.Output.Content,
		}.Build())
	}
	return sw.es.Send(res)
}

func (sw *streamWrapper) Recv() (*stream.Request, error) {
	req, err := sw.es.Recv()
	if err != nil {
		return nil, err
	}
	switch req.WhichRequest() {
	case pb.StreamRequest_ExecRequest_case:
		return &stream.Request{Request: convertPBStreamRequest(req.GetExecRequest())}, nil
	case pb.StreamRequest_ExecInput_case:
		return &stream.Request{Input: &stream.InputRequest{
			Index:   int(req.GetExecInput().GetIndex()),
			Fd:      int(req.GetExecInput().GetFd()),
			Content: req.GetExecInput().GetContent(),
		}}, nil
	case pb.StreamRequest_ExecResize_case:
		return &stream.Request{Resize: &stream.ResizeRequest{
			Index: int(req.GetExecResize().GetIndex()),
			Fd:    int(req.GetExecResize().GetFd()),
			Rows:  int(req.GetExecResize().GetRows()),
			Cols:  int(req.GetExecResize().GetCols()),
			X:     int(req.GetExecResize().GetX()),
			Y:     int(req.GetExecResize().GetY()),
		}}, nil
	case pb.StreamRequest_ExecCancel_case:
		return &stream.Request{Cancel: &struct{}{}}, nil
	}
	return nil, errors.ErrUnsupported
}

func convertPBStreamRequest(req *pb.Request) *model.Request {
	ret := &model.Request{
		RequestID: req.GetRequestID(),
	}
	for _, cmd := range req.GetCmd() {
		ret.Cmd = append(ret.Cmd, model.Cmd{
			Args:              cmd.GetArgs(),
			Env:               cmd.GetEnv(),
			TTY:               cmd.GetTty(),
			Files:             convertPBStreamFiles(cmd.GetFiles()),
			CPULimit:          cmd.GetCpuTimeLimit(),
			ClockLimit:        cmd.GetClockTimeLimit(),
			MemoryLimit:       cmd.GetMemoryLimit(),
			StackLimit:        cmd.GetStackLimit(),
			ProcLimit:         cmd.GetProcLimit(),
			CPURateLimit:      cmd.GetCpuRateLimit(),
			CPUSetLimit:       cmd.GetCpuSetLimit(),
			DataSegmentLimit:  cmd.GetDataSegmentLimit(),
			AddressSpaceLimit: cmd.GetAddressSpaceLimit(),
			CopyIn:            convertPBStreamCopyIn(cmd),
			CopyOut:           convertStreamCopyOut(cmd.GetCopyOut()),
			CopyOutCached:     convertStreamCopyOut(cmd.GetCopyOutCached()),
			CopyOutMax:        cmd.GetCopyOutMax(),
			CopyOutDir:        cmd.GetCopyOutDir(),
		})
	}
	for _, p := range req.GetPipeMapping() {
		ret.PipeMapping = append(ret.PipeMapping, model.PipeMap{
			In:    convertPBStreamPipeIndex(p.GetIn()),
			Out:   convertPBStreamPipeIndex(p.GetOut()),
			Max:   int64(p.GetMax()),
			Name:  p.GetName(),
			Proxy: p.GetProxy(),
		})
	}
	return ret
}

func convertPBStreamPipeIndex(pi *pb.Request_PipeMap_PipeIndex) model.PipeIndex {
	return model.PipeIndex{Index: int(pi.GetIndex()), Fd: int(pi.GetFd())}
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
	rt := make(map[string]model.CmdFile, len(cmd.GetCopyIn())+len(cmd.GetSymlinks()))
	for k, i := range cmd.GetCopyIn() {
		if !i.HasFile() {
			continue
		}
		rt[k] = convertPBStreamFile(i)
	}
	for k, v := range cmd.GetSymlinks() {
		rt[k] = model.CmdFile{Symlink: &v}
	}
	return rt
}

func convertPBStreamFile(i *pb.Request_File) model.CmdFile {
	switch i.WhichFile() {
	case pb.Request_File_Local_case:
		return model.CmdFile{Src: proto.String(i.GetLocal().GetSrc())}
	case pb.Request_File_Memory_case:
		s := byteArrayToString(i.GetMemory().GetContent())
		return model.CmdFile{Content: &s}
	case pb.Request_File_Cached_case:
		return model.CmdFile{FileID: proto.String(i.GetCached().GetFileID())}
	case pb.Request_File_Pipe_case:
		return model.CmdFile{Name: proto.String(i.GetPipe().GetName()), Max: proto.Int64(i.GetPipe().GetMax()), Pipe: i.GetPipe().GetPipe()}
	case pb.Request_File_StreamIn_case:
		return model.CmdFile{StreamIn: true}
	case pb.Request_File_StreamOut_case:
		return model.CmdFile{StreamOut: true}
	}
	return model.CmdFile{}
}

func convertStreamCopyOut(copyOut []*pb.Request_CmdCopyOutFile) []string {
	rt := make([]string, 0, len(copyOut))
	for _, n := range copyOut {
		name := n.GetName()
		if n.GetOptional() {
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
