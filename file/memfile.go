package file

import (
	"bytes"
	"fmt"
	"io"
)

var _ File = &memFile{}

// memFile represent a file like byte array
type memFile struct {
	name    string
	content []byte
}

// NewMemFile create a file interface from content and content should not be modified afterwards
func NewMemFile(name string, content []byte) File {
	return &memFile{
		name:    name,
		content: content,
	}
}

func (m *memFile) Name() string {
	return m.name
}

func (m *memFile) Content() ([]byte, error) {
	return m.content, nil
}

func (m *memFile) String() string {
	return fmt.Sprintf("[memfile:%v,%d]", m.name, len(m.content))
}

func (m *memFile) Reader() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(m.content)), nil
}
