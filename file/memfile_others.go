// +build !linux

package file

import (
	"errors"
	"os"
)

var errNotImplemented = errors.New("Memfile open is not defined on this platform")

func (m *memFile) Open() (*os.File, error) {
	return nil, errNotImplemented
}
