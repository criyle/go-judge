package file

import (
	"bytes"
	"os"

	"github.com/criyle/go-sandbox/pkg/memfd"
)

func (m *memFile) Open() (*os.File, error) {
	return memfd.DupToMemfd(m.name, bytes.NewReader(m.content))
}
