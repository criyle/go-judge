package grpcexecutor

import (
	"fmt"
	"io"
	"math"
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
	name   string
	r      io.ReadCloser
	w      *io.PipeWriter
	tty    *os.File
	done   chan struct{}
	hasTTY bool
}

type fileStreamInReader struct {
	*io.PipeReader
	fi *fileStreamIn
}

func (f *fileStreamInReader) TTY(tty *os.File) {
	f.fi.tty = tty
	close(f.fi.done)
}

func newFileStreamIn(name string, hasTTY bool) *fileStreamIn {
	r, w := io.Pipe()
	fi := &fileStreamIn{name: name, w: w, done: make(chan struct{}), hasTTY: hasTTY}
	fi.r = &fileStreamInReader{r, fi}
	return fi
}

func (f *fileStreamIn) GetTTY() *os.File {
	if !f.hasTTY {
		return nil
	}
	<-f.done
	return f.tty
}

func (f *fileStreamIn) Name() string {
	return f.name
}

func (f *fileStreamIn) Write(b []byte) (int, error) {
	return f.w.Write(b)
}

func (f *fileStreamIn) EnvFile(fs filestore.FileStore) (envexec.File, error) {
	return envexec.NewFileReader(f.r, true), nil
}

func (f *fileStreamIn) String() string {
	return fmt.Sprintf("fileStreamIn:%s", f.name)
}

func (f *fileStreamIn) Close() error {
	f.r.Close()
	return f.w.Close()
}

type fileStreamOut struct {
	name string
	r    *io.PipeReader
	w    *io.PipeWriter
}

func newFileStreamOut(name string) *fileStreamOut {
	r, w := io.Pipe()
	return &fileStreamOut{name: name, r: r, w: w}
}

func (f *fileStreamOut) Name() string {
	return f.name
}

func (f *fileStreamOut) Read(b []byte) (int, error) {
	return f.r.Read(b)
}

func (f *fileStreamOut) EnvFile(fs filestore.FileStore) (envexec.File, error) {
	return envexec.NewFileWriter(f.w, envexec.Size(math.MaxInt32)), nil
}

func (f *fileStreamOut) String() string {
	return fmt.Sprintf("fileStreamOut:%s", f.name)
}

func (f *fileStreamOut) Close() error {
	f.w.Close()
	return f.r.Close()
}
