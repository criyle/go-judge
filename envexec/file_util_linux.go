package envexec

import (
	"io"
	"os"
	"sync/atomic"

	"github.com/criyle/go-sandbox/pkg/memfd"
)

const memfdName = "input"

var enableMemFd atomic.Int32

func readerToFile(reader io.Reader) (*os.File, error) {
	if enableMemFd.Load() == 0 {
		f, err := memfd.DupToMemfd(memfdName, reader)
		if err == nil {
			return f, err
		}
		enableMemFd.Store(1)
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	go func() {
		defer w.Close()
		w.ReadFrom(reader)
	}()
	return r, nil
}
