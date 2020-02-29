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

// File defines file name with its content
// file could on file system or memory
type File interface {
	Opener
	Content() ([]byte, error)
	Name() string
	Reader() (io.ReadCloser, error)
}
