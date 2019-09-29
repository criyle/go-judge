package memfile

import (
	"bytes"
	"os"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-sandbox/pkg/memfd"
)

// File represent a file like byte array
type File struct {
	name    string
	content []byte
}

// New create a file interface from content and content should not be modified afterwards
func New(name string, content []byte) file.File {
	return &File{
		name:    name,
		content: content,
	}
}

// Name returns the file name
func (m *File) Name() string {
	return m.name
}

// Content returns the file content
func (m *File) Content() ([]byte, error) {
	return m.content, nil
}

// Open creates a memfd file
func (m *File) Open() (*os.File, error) {
	return memfd.DupToMemfd(m.name, bytes.NewReader(m.content))
}
