package file

import "os"

// File defines file name with its content
// file could on file system or memory
type File interface {
	// Name get the file name
	Name() string

	// Content reads the file content
	Content() ([]byte, error)

	// Open opens the file in readonly mode
	// caller should close afterwards
	Open() (*os.File, error)
}
