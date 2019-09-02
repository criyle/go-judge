package localfile

import (
	"io/ioutil"
	"os"
)

// File stores a path to represent a local file
type File struct {
	path string
}

// New creats a wrapper to path
func New(path string) *File {
	return &File{
		path: path,
	}
}

// Name get the path
func (f *File) Name() string {
	return f.path
}

// Content reads file content
func (f *File) Content() ([]byte, error) {
	return ioutil.ReadFile(f.path)
}

// Open opens the file
func (f *File) Open() (*os.File, error) {
	return os.Open(f.path)
}
