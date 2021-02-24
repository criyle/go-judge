package file

import (
	"fmt"
	"io"
	"os"
)

var _ File = &localFile{}

// localFile stores a path to represent a local file
type localFile struct {
	name, path string
}

// NewLocalFile creates a wrapper to file system by path
func NewLocalFile(name, path string) File {
	return &localFile{
		name: name,
		path: path,
	}
}

func (f *localFile) Name() string {
	return f.name
}

func (f *localFile) Content() ([]byte, error) {
	return os.ReadFile(f.path)
}

func (f *localFile) Open() (*os.File, error) {
	return os.Open(f.path)
}

func (f *localFile) String() string {
	return fmt.Sprintf("[localfile:%v(%v)]", f.path, f.name)
}

func (f *localFile) Reader() (io.ReadCloser, error) {
	return os.Open(f.path)
}
