package file

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/criyle/go-sandbox/pkg/memfd"
)

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

func (m *memFile) Open() (*os.File, error) {
	return memfd.DupToMemfd(m.name, bytes.NewReader(m.content))
}

func (m *memFile) String() string {
	return fmt.Sprintf("[memfile:%v,%d]", m.name, len(m.content))
}

func (m *memFile) Reader() (io.ReadCloser, error) {
	return ioutil.NopCloser(bytes.NewReader(m.content)), nil
}
