package stream

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
	_ worker.CmdFile    = &fileStreamIn{}
	_ worker.CmdFile    = &fileStreamOut{}
	_ envexec.ReaderTTY = &fileStreamInReader{}
)

type fileStreamIn struct {
	index  int
	fd     int
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

func newFileStreamIn(index, fd int, hasTTY bool) *fileStreamIn {
	r, w := io.Pipe()
	fi := &fileStreamIn{index: index, fd: fd, w: w, done: make(chan struct{}), hasTTY: hasTTY}
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

func (f *fileStreamIn) Write(b []byte) (int, error) {
	return f.w.Write(b)
}

func (f *fileStreamIn) EnvFile(fs filestore.FileStore) (envexec.File, error) {
	return envexec.NewFileReader(f.r, true), nil
}

func (f *fileStreamIn) String() string {
	return fmt.Sprintf("fileStreamIn:(index:%d,fd:%d)", f.index, f.fd)
}

func (f *fileStreamIn) Close() error {
	f.r.Close()
	return f.w.Close()
}

type fileStreamOut struct {
	index int
	fd    int
	r     *io.PipeReader
	w     *io.PipeWriter
}

func newFileStreamOut(index, fd int) *fileStreamOut {
	r, w := io.Pipe()
	return &fileStreamOut{index: index, fd: fd, r: r, w: w}
}

func (f *fileStreamOut) Read(b []byte) (int, error) {
	return f.r.Read(b)
}

func (f *fileStreamOut) EnvFile(fs filestore.FileStore) (envexec.File, error) {
	return envexec.NewFileWriter(f.w, envexec.Size(math.MaxInt32)), nil
}

func (f *fileStreamOut) String() string {
	return fmt.Sprintf("fileStreamOut:(index:%d,fd:%d)", f.index, f.fd)
}

func (f *fileStreamOut) Close() error {
	f.w.Close()
	return f.r.Close()
}
