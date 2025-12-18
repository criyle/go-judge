//go:build !linux

package envexec

import (
	"io"
	"os"
)

func readerToFile(reader io.Reader) (*os.File, error) {
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
