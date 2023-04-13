package envexec

import (
	"fmt"
	"io"
	"os"
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

// NewFileReader creates File input which can be fully read before exec
// or piped into exec
func NewFileReader(r io.Reader, s bool) File {
	return &FileReader{Reader: r, Stream: s}
}

// ReaderTTY will be asserts when File Reader is provided and TTY is enabled
// and then TTY will be called with pty file
type ReaderTTY interface {
	TTY(*os.File)
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
		return nil, fmt.Errorf("file cannot open as reader %v", f)
	}
}
