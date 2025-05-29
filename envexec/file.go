package envexec

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/creack/pty"
)

var (
	_ File = &FileReader{}
	_ File = &fileStreamIn{}
	_ File = &fileStreamOut{}
	_ File = &FileInput{}
	_ File = &FileCollector{}
	_ File = &FileWriter{}
	_ File = &FileOpened{}
)

// File defines interface of envexec files
type File interface {
	isFile()
}

// FileReader represent file input which can be fully read before exec
// or piped into exec
type FileReader struct {
	Reader io.Reader
	Stream bool
}

func (*FileReader) isFile() {}

// NewFileReader creates File input which can be fully read before exec.
// If pipe is required, use the FileStream to get the write end of pipe instead
func NewFileReader(r io.Reader) File {
	return &FileReader{Reader: r}
}

// FileStreamIn represent a input streaming pipe and the streamer is able to write
// to the write end of the pipe after pipe created. It is the callers
// responsibility to close the WritePipe
type FileStreamIn interface {
	File
	io.WriteCloser
	SetSize(*TerminalSize) error
}

// TerminalSize controls the size of the terminal if TTY is enabled
type TerminalSize struct {
	Rows uint16 // ws_row: Number of rows (in cells).
	Cols uint16 // ws_col: Number of columns (in cells).
	X    uint16 // ws_xpixel: Width in pixels.
	Y    uint16 // ws_ypixel: Height in pixels.
}

type fileStreamIn struct {
	started chan struct{}
	closed  chan struct{}
	w       *sharedFile
}

func NewFileStreamIn() FileStreamIn {
	return &fileStreamIn{
		started: make(chan struct{}),
		closed:  make(chan struct{}),
	}
}

func (*fileStreamIn) isFile() {}

func (f *fileStreamIn) start(w *sharedFile) {
	select {
	case <-f.closed:
		w.Close()

	default:
		f.w = w
		close(f.started)
	}
}

func (f *fileStreamIn) Write(b []byte) (int, error) {
	select {
	case <-f.started:
		return f.w.f.Write(b)

	case <-f.closed:
		return 0, io.EOF
	}
}

func (f *fileStreamIn) SetSize(s *TerminalSize) error {
	select {
	case <-f.started:
		return pty.Setsize(f.w.f, &pty.Winsize{
			Rows: s.Rows,
			Cols: s.Cols,
			X:    s.X,
			Y:    s.Y,
		})

	case <-f.closed:
		return io.ErrClosedPipe
	}
}

func (f *fileStreamIn) Close() error {
	select {
	case <-f.started:
		close(f.closed)
		return f.w.Close()

	case <-f.closed:
		return io.ErrClosedPipe

	default:
		close(f.closed)
		return nil
	}
}

// FileStreamOut represent a out streaming pipe and the streamer is able to read
// to the read end of the pipe after pipe created. It is the callers
// responsibility to close the ReadPipe
type FileStreamOut interface {
	File
	io.ReadCloser
}

type fileStreamOut struct {
	started chan struct{}
	closed  chan struct{}
	r       *sharedFile
}

func NewFileStreamOut() FileStreamOut {
	return &fileStreamOut{
		started: make(chan struct{}),
		closed:  make(chan struct{}),
	}
}

func (*fileStreamOut) isFile() {}

func (f *fileStreamOut) start(r *sharedFile) {
	select {
	case <-f.closed:
		r.Close()

	default:
		f.r = r
		close(f.started)
	}
}

func (f *fileStreamOut) Read(p []byte) (n int, err error) {
	select {
	case <-f.started:
		return f.r.f.Read(p)

	case <-f.closed:
		return 0, io.EOF
	}
}

func (f *fileStreamOut) Close() error {
	select {
	case <-f.started:
		close(f.closed)
		return f.r.Close()

	case <-f.closed:
		return io.ErrClosedPipe

	default:
		close(f.closed)
		return nil
	}
}

// FileInput represent file input which will be opened in read-only mode
type FileInput struct {
	Path string
}

func (*FileInput) isFile() {}

// NewFileInput creates file input which will be opened in read-only mode
func NewFileInput(p string) File {
	return &FileInput{Path: p}
}

// FileCollector represent pipe output which will be collected through pipe
type FileCollector struct {
	Name  string
	Limit Size
	Pipe  bool
}

func (*FileCollector) isFile() {}

// NewFileCollector creates file output which will be collected through pipe
func NewFileCollector(name string, limit Size, pipe bool) File {
	return &FileCollector{Name: name, Limit: limit, Pipe: pipe}
}

// FileWriter represent pipe output which will be piped out from exec
type FileWriter struct {
	Writer io.Writer
	Limit  Size
}

func (*FileWriter) isFile() {}

// NewFileWriter create File which will be piped out from exec
func NewFileWriter(w io.Writer, limit Size) File {
	return &FileWriter{Writer: w, Limit: limit}
}

// FileOpened represent file that is already opened
type FileOpened struct {
	File *os.File
}

func (*FileOpened) isFile() {}

// NewFileOpened creates file that contains already opened file and it will be closed
func NewFileOpened(f *os.File) File {
	return &FileOpened{File: f}
}

// FileToReader get a Reader from underlying file
// the reader need to be closed by caller explicitly
func FileToReader(f File) (io.ReadCloser, error) {
	switch f := f.(type) {
	case *FileOpened:
		return f.File, nil

	case *FileReader:
		return io.NopCloser(f.Reader), nil

	case *FileInput:
		file, err := os.Open(f.Path)
		if err != nil {
			return nil, err
		}
		return file, nil

	default:
		return nil, fmt.Errorf("file cannot open as reader: %T", f)
	}
}

type sharedFile struct {
	mu    sync.Mutex
	f     *os.File
	count int
}

func newSharedFile(f *os.File) *sharedFile {
	return &sharedFile{f: f, count: 0}
}

func (s *sharedFile) Acquire() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
}

func (s *sharedFile) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count--
	if s.count == 0 {
		return s.f.Close()
	}
	return nil
}
