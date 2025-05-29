package stream

import (
	"fmt"
	"os"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/worker"
)

var (
	_ worker.CmdFile = &fileStreamIn{}
	_ worker.CmdFile = &fileStreamOut{}
)

type fileStreamIn struct {
	stream envexec.FileStreamIn
	index  int
	fd     int
	hasTTY bool
}

func newFileStreamIn(index, fd int, hasTTY bool) *fileStreamIn {
	return &fileStreamIn{
		stream: envexec.NewFileStreamIn(),
		index:  index,
		fd:     fd,
		hasTTY: hasTTY,
	}
}

func (f *fileStreamIn) GetTTY() *os.File {
	if !f.hasTTY {
		return nil
	}
	return f.stream.WritePipe()
}

func (f *fileStreamIn) Write(b []byte) (int, error) {
	return f.stream.WritePipe().Write(b)
}

func (f *fileStreamIn) EnvFile(fs filestore.FileStore) (envexec.File, error) {
	return f.stream, nil
}

func (f *fileStreamIn) String() string {
	return fmt.Sprintf("fileStreamIn:(index:%d,fd:%d)", f.index, f.fd)
}

func (f *fileStreamIn) Close() error {
	return f.stream.Close()
}

type fileStreamOut struct {
	stream envexec.FileStreamOut
	index  int
	fd     int
}

func newFileStreamOut(index, fd int) *fileStreamOut {
	return &fileStreamOut{
		stream: envexec.NewFileStreamOut(),
		index:  index,
		fd:     fd,
	}
}

func (f *fileStreamOut) Read(b []byte) (int, error) {
	return f.stream.ReadPipe().Read(b)
}

func (f *fileStreamOut) EnvFile(fs filestore.FileStore) (envexec.File, error) {
	return f.stream, nil
}

func (f *fileStreamOut) String() string {
	return fmt.Sprintf("fileStreamOut:(index:%d,fd:%d)", f.index, f.fd)
}

func (f *fileStreamOut) Close() error {
	return f.stream.Close()
}
