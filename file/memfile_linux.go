package file

import (
	"bytes"
	"os"

	"github.com/criyle/go-sandbox/pkg/memfd"
)

var enableMemFd = true

func (m *memFile) Open() (*os.File, error) {
	if enableMemFd {
		f, err := memfd.DupToMemfd(m.name, bytes.NewReader(m.content))
		if err == nil {
			return f, err
		}
		enableMemFd = false
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	go func() {
		defer w.Close()
		w.Write(m.content)
	}()
	return r, nil
}
