package file

import (
	"io"
	"os"
)

// Opener opens the file in readonly mode
// caller should close afterwards
type Opener interface {
	Open() (*os.File, error)
}

// ReaderOpener creates readCloser for caller
type ReaderOpener interface {
	Reader() (io.ReadCloser, error)
}

// File defines file name with its content
// file could on file system or memory
type File interface {
	Opener
	ReaderOpener
	Content() ([]byte, error)
	Name() string
}
