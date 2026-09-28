package grpcexecutor

import (
	"io"
	"os"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const fileStreamChunkSize = 1 << 20

func (e *execServer) FileGetStream(f *pb.FileID, stream pb.Executor_FileGetStreamServer) error {
	name, file := e.fs.Get(f.GetFileID())
	if file == nil {
		return status.Errorf(codes.NotFound, "file not found: %q", f.GetFileID())
	}
	r, err := envexec.FileToReader(file)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	defer r.Close()

	buf := make([]byte, fileStreamChunkSize)
	first := true
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			chunkName := ""
			if first {
				chunkName = name
				first = false
			}
			if err := stream.Send(pb.FileContent_builder{
				Name:    chunkName,
				Content: buf[:n],
			}.Build()); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return status.Error(codes.Internal, readErr.Error())
		}
	}

	if first {
		return stream.Send(pb.FileContent_builder{Name: name}.Build())
	}
	return nil
}

func (e *execServer) FileAddStream(stream pb.Executor_FileAddStreamServer) error {
	first, err := stream.Recv()
	if err == io.EOF {
		return status.Error(codes.InvalidArgument, "file upload stream is empty")
	}
	if err != nil {
		return err
	}

	f, err := e.fs.New()
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	path := f.Name()
	closed := false
	committed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
		if !committed {
			_ = os.Remove(path)
		}
	}()

	name := first.GetName()
	if err := writeFileStreamChunk(f, first.GetContent()); err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	for {
		chunk, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			return recvErr
		}
		if chunkName := chunk.GetName(); chunkName != "" && chunkName != name {
			return status.Errorf(codes.InvalidArgument, "file name changed during upload: %q -> %q", name, chunkName)
		}
		if err := writeFileStreamChunk(f, chunk.GetContent()); err != nil {
			return status.Error(codes.Internal, err.Error())
		}
	}

	if err := f.Close(); err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	closed = true

	fid, err := e.fs.Add(name, path)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	if err := stream.SendAndClose(pb.FileID_builder{FileID: fid}.Build()); err != nil {
		e.fs.Remove(fid)
		return err
	}
	committed = true
	return nil
}

func writeFileStreamChunk(f *os.File, content []byte) error {
	for len(content) > 0 {
		n, err := f.Write(content)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		content = content[n:]
	}
	return nil
}
