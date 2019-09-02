package file

import "os"

// File defines file name with its content
// file could on file system or memory
type File interface {
	Name() string
	Content() ([]byte, error) // get content of the file
	Open() (*os.File, error)  // get readonly fd of the file
}
