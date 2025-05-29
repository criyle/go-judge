package envexec

import (
	"fmt"
	"io"
	"os"
	"sync"
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
	Done() <-chan struct{}
	WritePipe() *os.File
	Close() error
}

type fileStreamIn struct {
	done chan struct{}
	w    *sharedFile
}

func NewFileStreamIn() FileStreamIn {
	return &fileStreamIn{
		done: make(chan struct{}),
	}
}

func (*fileStreamIn) isFile() {}

func (f *fileStreamIn) start(w *sharedFile) {
	f.w = w
	close(f.done)
}

func (f *fileStreamIn) Done() <-chan struct{} {
	return f.done
}

func (f *fileStreamIn) WritePipe() *os.File {
	<-f.done
	return f.w.f
}

func (f *fileStreamIn) Close() error {
	if f.w != nil {
		return f.w.Close()
	}
	return nil
}

// FileStreamOut represent a out streaming pipe and the streamer is able to read
// to the read end of the pipe after pipe created. It is the callers
// responsibility to close the ReadPipe
type FileStreamOut interface {
	File
	Done() <-chan struct{}
	ReadPipe() *os.File
	Close() error
}

type fileStreamOut struct {
	done chan struct{}
	r    *sharedFile
}

func NewFileStreamOut() FileStreamOut {
	return &fileStreamOut{
		done: make(chan struct{}),
	}
}

func (*fileStreamOut) isFile() {}

func (f *fileStreamOut) start(r *sharedFile) {
	f.r = r
	close(f.done)
}

func (f *fileStreamOut) Done() <-chan struct{} {
	return f.done
}

func (f *fileStreamOut) ReadPipe() *os.File {
	<-f.done
	return f.r.f
}

func (f *fileStreamOut) Close() error {
	if f.r != nil {
		return f.r.Close()
	}
	return nil
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

func newShreadFile(f *os.File) *sharedFile {
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
