package grpcexecutor

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"testing"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type fileAddStreamStub struct {
	grpc.ServerStream
	chunks  []*pb.FileContent
	recvErr error
	sendErr error
	pos     int
	result  *pb.FileID
}

func (s *fileAddStreamStub) Recv() (*pb.FileContent, error) {
	if s.pos < len(s.chunks) {
		chunk := s.chunks[s.pos]
		s.pos++
		return chunk, nil
	}
	if s.recvErr != nil {
		err := s.recvErr
		s.recvErr = nil
		return nil, err
	}
	return nil, io.EOF
}

func (s *fileAddStreamStub) SendAndClose(result *pb.FileID) error {
	s.result = result
	return s.sendErr
}

type fileGetStreamStub struct {
	grpc.ServerStream
	sent    []*pb.FileContent
	sendErr error
}

func (s *fileGetStreamStub) Send(content *pb.FileContent) error {
	if s.sendErr != nil {
		return s.sendErr
	}
	s.sent = append(s.sent, content)
	return nil
}

func newFileStreamTestServer(t *testing.T) (*execServer, filestore.FileStore, string) {
	t.Helper()
	dir := t.TempDir()
	fs := filestore.NewFileLocalStore(dir)
	return &execServer{fs: fs}, fs, dir
}

func addFileToStore(t *testing.T, fs filestore.FileStore, name string, content []byte) string {
	t.Helper()
	f, err := fs.New()
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	fid, err := fs.Add(name, path)
	if err != nil {
		t.Fatal(err)
	}
	return fid
}

func assertStoreDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files in store, found %d", len(entries))
	}
}

func TestFileStreamGRPCRoundTrip(t *testing.T) {
	dir := t.TempDir()
	fs := filestore.NewFileLocalStore(dir)
	listener := bufconn.Listen(4 * fileStreamChunkSize)
	grpcServer := grpc.NewServer()
	pb.RegisterExecutorServer(grpcServer, &execServer{fs: fs})
	go func() {
		_ = grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	client := pb.NewExecutorClient(conn)
	want := bytes.Repeat([]byte("abcdef"), (2*fileStreamChunkSize+123)/6+1)
	want = want[:2*fileStreamChunkSize+123]

	upload, err := client.FileAddStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cut1 := 700000
	cut2 := 1700000
	for i, chunk := range [][]byte{want[:cut1], want[cut1:cut2], want[cut2:]} {
		name := ""
		if i == 0 {
			name = "artifact.bin"
		}
		if err := upload.Send(pb.FileContent_builder{Name: name, Content: chunk}.Build()); err != nil {
			t.Fatal(err)
		}
	}
	fid, err := upload.CloseAndRecv()
	if err != nil {
		t.Fatal(err)
	}
	if fid.GetFileID() == "" {
		t.Fatal("expected a file ID")
	}

	download, err := client.FileGetStream(context.Background(), fid)
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	chunks := 0
	for {
		chunk, err := download.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if chunks == 0 {
			if chunk.GetName() != "artifact.bin" {
				t.Fatalf("expected first chunk name artifact.bin, got %q", chunk.GetName())
			}
		} else if chunk.GetName() != "" {
			t.Fatalf("expected subsequent chunk name to be empty, got %q", chunk.GetName())
		}
		got.Write(chunk.GetContent())
		chunks++
	}
	if chunks < 3 {
		t.Fatalf("expected at least 3 download chunks, got %d", chunks)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("downloaded content does not match uploaded content")
	}
}

func TestFileGetStreamEmptyFile(t *testing.T) {
	server, fs, _ := newFileStreamTestServer(t)
	fid := addFileToStore(t, fs, "empty.txt", nil)
	stream := &fileGetStreamStub{}

	if err := server.FileGetStream(pb.FileID_builder{FileID: fid}.Build(), stream); err != nil {
		t.Fatal(err)
	}
	if len(stream.sent) != 1 {
		t.Fatalf("expected one name-only chunk, got %d", len(stream.sent))
	}
	if stream.sent[0].GetName() != "empty.txt" || len(stream.sent[0].GetContent()) != 0 {
		t.Fatalf("unexpected empty-file response: name=%q content=%d bytes", stream.sent[0].GetName(), len(stream.sent[0].GetContent()))
	}
}

func TestFileGetStreamNotFound(t *testing.T) {
	server, _, _ := newFileStreamTestServer(t)
	err := server.FileGetStream(pb.FileID_builder{FileID: "missing"}.Build(), &fileGetStreamStub{})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestFileAddStreamRejectsEmptyStream(t *testing.T) {
	server, _, dir := newFileStreamTestServer(t)
	err := server.FileAddStream(&fileAddStreamStub{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
	assertStoreDirEmpty(t, dir)
}

func TestFileAddStreamRejectsNameChange(t *testing.T) {
	server, _, dir := newFileStreamTestServer(t)
	stream := &fileAddStreamStub{chunks: []*pb.FileContent{
		pb.FileContent_builder{Name: "a.bin", Content: []byte("a")}.Build(),
		pb.FileContent_builder{Name: "b.bin", Content: []byte("b")}.Build(),
	}}

	err := server.FileAddStream(stream)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
	assertStoreDirEmpty(t, dir)
}

func TestFileAddStreamCleansUpOnReceiveError(t *testing.T) {
	server, _, dir := newFileStreamTestServer(t)
	stream := &fileAddStreamStub{
		chunks: []*pb.FileContent{
			pb.FileContent_builder{Name: "artifact.bin", Content: []byte("partial")}.Build(),
		},
		recvErr: context.Canceled,
	}

	err := server.FileAddStream(stream)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	assertStoreDirEmpty(t, dir)
}

func TestFileAddStreamRollsBackOnSendFailure(t *testing.T) {
	server, fs, dir := newFileStreamTestServer(t)
	sendErr := errors.New("send failed")
	stream := &fileAddStreamStub{
		chunks: []*pb.FileContent{
			pb.FileContent_builder{Name: "artifact.bin", Content: []byte("complete")}.Build(),
		},
		sendErr: sendErr,
	}

	err := server.FileAddStream(stream)
	if !errors.Is(err, sendErr) {
		t.Fatalf("expected send failure, got %v", err)
	}
	if got := len(fs.List()); got != 0 {
		t.Fatalf("expected no published files, found %d", got)
	}
	assertStoreDirEmpty(t, dir)
}

func TestFileGetStreamPropagatesSendFailure(t *testing.T) {
	server, fs, _ := newFileStreamTestServer(t)
	fid := addFileToStore(t, fs, "artifact.bin", []byte("content"))
	sendErr := errors.New("send failed")
	stream := &fileGetStreamStub{sendErr: sendErr}

	err := server.FileGetStream(pb.FileID_builder{FileID: fid}.Build(), stream)
	if !errors.Is(err, sendErr) {
		t.Fatalf("expected send failure, got %v", err)
	}
	name, file := fs.Get(fid)
	if name != "artifact.bin" || file == nil {
		t.Fatal("download send failure must not delete the stored file")
	}
	r, err := envexec.FileToReader(file)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
}
