package grpcexecutor

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/criyle/go-judge/cmd/go-judge/model"
	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/pb"
	"github.com/criyle/go-judge/worker"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// New creates grpc executor server
func New(worker worker.Worker, fs filestore.FileStore, srcPrefix []string, logger *zap.Logger) pb.ExecutorServer {
	return &execServer{
		worker:    worker,
		fs:        fs,
		srcPrefix: srcPrefix,
		logger:    logger,
	}
}

type execServer struct {
	pb.UnimplementedExecutorServer
	worker    worker.Worker
	fs        filestore.FileStore
	srcPrefix []string
	logger    *zap.Logger
}

func (e *execServer) Exec(ctx context.Context, req *pb.Request) (*pb.Response, error) {
	r, err := convertPBRequest(req, e.srcPrefix)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	e.logger.Sugar().Debugf("request: %+v", r)
	rtCh, _ := e.worker.Submit(ctx, r)
	rt := <-rtCh
	e.logger.Sugar().Debugf("response: %+v", rt)
	if rt.Error != nil {
		return nil, status.Error(codes.Internal, rt.Error.Error())
	}
	ret, err := model.ConvertResponse(rt, false)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	resp, err := convertPBResponse(ret)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return resp, nil
}

func (e *execServer) FileList(c context.Context, n *emptypb.Empty) (*pb.FileListType, error) {
	return &pb.FileListType{
		FileIDs: e.fs.List(),
	}, nil
}

func (e *execServer) FileGet(c context.Context, f *pb.FileID) (*pb.FileContent, error) {
	name, file := e.fs.Get(f.GetFileID())
	if file == nil {
		return nil, status.Errorf(codes.NotFound, "file %v not found", f.GetFileID())
	}
	r, err := envexec.FileToReader(file)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer r.Close()

	content, err := io.ReadAll(r)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.FileContent{
		Name:    name,
		Content: content,
	}, nil
}

func (e *execServer) FileAdd(c context.Context, fc *pb.FileContent) (*pb.FileID, error) {
	f, err := e.fs.New()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer f.Close()

	if _, err := f.Write(fc.GetContent()); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	fid, err := e.fs.Add(fc.GetName(), f.Name())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.FileID{
		FileID: fid,
	}, nil
}

func (e *execServer) FileDelete(c context.Context, f *pb.FileID) (*emptypb.Empty, error) {
	ok := e.fs.Remove(f.GetFileID())
	if !ok {
		return nil, status.Errorf(codes.NotFound, "file id does not exists for %v", f.GetFileID())
	}
	return &emptypb.Empty{}, nil
}

func convertPBResponse(r model.Response) (*pb.Response, error) {
	res := &pb.Response{
		RequestID: r.RequestID,
		Results:   make([]*pb.Response_Result, 0, len(r.Results)),
		Error:     r.ErrorMsg,
	}
	for _, c := range r.Results {
		rt, err := convertPBResult(c)
		if err != nil {
			return nil, err
		}
		res.Results = append(res.Results, rt)
	}
	return res, nil
}

func convertPBResult(r model.Result) (*pb.Response_Result, error) {
	return &pb.Response_Result{
		Status:     pb.Response_Result_StatusType(r.Status),
		ExitStatus: int32(r.ExitStatus),
		Error:      r.Error,
		Time:       uint64(r.Time),
		RunTime:    uint64(r.RunTime),
		Memory:     uint64(r.Memory),
		Files:      r.Buffs,
		FileIDs:    r.FileIDs,
		FileError:  convertPBFileError(r.FileError),
	}, nil
}

func convertPBFileError(fe []envexec.FileError) []*pb.Response_FileError {
	rt := make([]*pb.Response_FileError, 0, len(fe))
	for _, e := range fe {
		rt = append(rt, &pb.Response_FileError{
			Name:    e.Name,
			Type:    pb.Response_FileError_ErrorType(e.Type),
			Message: e.Message,
		})
	}
	return rt
}

func convertPBRequest(r *pb.Request, srcPrefix []string) (req *worker.Request, err error) {
	req = &worker.Request{
		RequestID:   r.RequestID,
		Cmd:         make([]worker.Cmd, 0, len(r.Cmd)),
		PipeMapping: make([]worker.PipeMap, 0, len(r.PipeMapping)),
	}
	for _, c := range r.Cmd {
		cm, err := convertPBCmd(c, srcPrefix)
		if err != nil {
			return nil, err
		}
		req.Cmd = append(req.Cmd, cm)
	}
	for _, p := range r.PipeMapping {
		pm := convertPBPipeMap(p)
		req.PipeMapping = append(req.PipeMapping, pm)
	}
	return req, nil
}

func convertPBPipeMap(p *pb.Request_PipeMap) worker.PipeMap {
	return worker.PipeMap{
		In:    convertPBPipeIndex(p.GetIn()),
		Out:   convertPBPipeIndex(p.GetOut()),
		Proxy: p.GetProxy(),
		Name:  p.GetName(),
		Limit: worker.Size(p.Max),
	}
}

func convertPBPipeIndex(p *pb.Request_PipeMap_PipeIndex) worker.PipeIndex {
	return worker.PipeIndex{Index: int(p.GetIndex()), Fd: int(p.GetFd())}
}

func convertPBCmd(c *pb.Request_CmdType, srcPrefix []string) (cm worker.Cmd, err error) {
	cm = worker.Cmd{
		Args:              c.GetArgs(),
		Env:               c.GetEnv(),
		TTY:               c.GetTty(),
		CPULimit:          time.Duration(c.GetCpuTimeLimit()),
		ClockLimit:        time.Duration(c.GetClockTimeLimit()),
		MemoryLimit:       envexec.Size(c.GetMemoryLimit()),
		StackLimit:        envexec.Size(c.GetStackLimit()),
		ProcLimit:         c.GetProcLimit(),
		CPURateLimit:      c.GetCpuRateLimit(),
		CPUSetLimit:       c.GetCpuSetLimit(),
		DataSegmentLimit:  c.GetDataSegmentLimit(),
		AddressSpaceLimit: c.GetAddressSpaceLimit(),
		CopyOut:           convertCopyOut(c.GetCopyOut()),
		CopyOutCached:     convertCopyOut(c.GetCopyOutCached()),
		CopyOutMax:        c.GetCopyOutMax(),
		CopyOutDir:        c.GetCopyOutDir(),
		Symlinks:          c.GetSymlinks(),
	}
	for _, f := range c.GetFiles() {
		cf, err := convertPBFile(f, srcPrefix)
		if err != nil {
			return cm, err
		}
		cm.Files = append(cm.Files, cf)
	}
	if copyIn := c.GetCopyIn(); copyIn != nil {
		cm.CopyIn = make(map[string]worker.CmdFile)
		for k, f := range copyIn {
			cf, err := convertPBFile(f, srcPrefix)
			if err != nil {
				return cm, err
			}
			cm.CopyIn[k] = cf
		}
	}
	return cm, nil
}

func convertPBFile(c *pb.Request_File, srcPrefix []string) (worker.CmdFile, error) {
	switch c := c.File.(type) {
	case nil:
		return nil, nil
	case *pb.Request_File_Local:
		if len(srcPrefix) > 0 {
			ok, err := model.CheckPathPrefixes(c.Local.GetSrc(), srcPrefix)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("file (%s) does not under (%s)", c.Local.GetSrc(), srcPrefix)
			}
		}
		return &worker.LocalFile{Src: c.Local.GetSrc()}, nil
	case *pb.Request_File_Memory:
		return &worker.MemoryFile{Content: c.Memory.GetContent()}, nil
	case *pb.Request_File_Cached:
		return &worker.CachedFile{FileID: c.Cached.GetFileID()}, nil
	case *pb.Request_File_Pipe:
		return &worker.Collector{Name: c.Pipe.GetName(), Max: envexec.Size(c.Pipe.GetMax()), Pipe: c.Pipe.GetPipe()}, nil
	}
	return nil, fmt.Errorf("request file type not supported yet %v", c)
}

func convertCopyOut(copyOut []*pb.Request_CmdCopyOutFile) []worker.CmdCopyOutFile {
	rt := make([]worker.CmdCopyOutFile, 0, len(copyOut))
	for _, n := range copyOut {
		rt = append(rt, worker.CmdCopyOutFile{
			Name:     n.GetName(),
			Optional: n.GetOptional(),
		})
	}
	return rt
}
