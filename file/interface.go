package file

import "os"

// Opener opens the file in readonly mode
// caller should close afterwards
type Opener interface {
	Open() (*os.File, error)
}

// Contenter reads the file content
type Contenter interface {
	Content() ([]byte, error)
}

// Namer get the file name
type Namer interface {
	Name() string
}

// File defines file name with its content
// file could on file system or memory
type File interface {
	Opener
	Contenter
	Namer
}
