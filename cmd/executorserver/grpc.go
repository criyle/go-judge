package main

import (
	"context"
	"fmt"

	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/pb"
	"github.com/criyle/go-judge/worker"
)

type execServer struct {
	pb.UnimplementedExecutorServer
	fs filestore.FileStore
}

func (e *execServer) Exec(ctx context.Context, req *pb.Request) (*pb.Response, error) {
	r, err := convertPBRequest(req)
	if err != nil {
		return nil, err
	}
	rt := <-work.Submit(r)
	if rt.Error != nil {
		return nil, err
	}
	return convertPBResponse(rt), nil
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
		Memory:     r.Memory,
		Files:      r.Files,
		FileIDs:    r.FileIDs,
	}
}

func convertPBRequest(r *pb.Request) (*worker.Request, error) {
	req := &worker.Request{
		RequestID:   r.RequestID,
		Cmd:         make([]worker.Cmd, 0, len(r.Cmd)),
		PipeMapping: make([]worker.PipeMap, 0, len(r.PipeMapping)),
	}
	for _, c := range r.Cmd {
		cm, err := convertPBCmd(c)
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

func convertPBCmd(c *pb.Request_CmdType) (worker.Cmd, error) {
	cm := worker.Cmd{
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
	for _, f := range c.GetFiles() {
		cf, err := convertPBFile(f)
		if err != nil {
			return cm, err
		}
		cm.Files = append(cm.Files, cf)
	}
	if copyIn := c.GetCopyIn(); copyIn != nil {
		cm.CopyIn = make(map[string]worker.CmdFile)
		for k, f := range copyIn {
			cf, err := convertPBFile(f)
			if err != nil {
				return cm, err
			}
			cm.CopyIn[k] = cf
		}
	}
	return cm, nil
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
