package localfile

import (
	"io/ioutil"
	"os"

	"github.com/criyle/go-judge/file"
)

// File stores a path to represent a local file
type File struct {
	name, path string
}

// New creates a wrapper to file system by path
func New(name, path string) file.File {
	return &File{
		name: name,
		path: path,
	}
}

// Name get the path
func (f *File) Name() string {
	return f.name
}

// Content reads file content
func (f *File) Content() ([]byte, error) {
	return ioutil.ReadFile(f.path)
}

// Open opens the file
func (f *File) Open() (*os.File, error) {
	return os.Open(f.path)
}
