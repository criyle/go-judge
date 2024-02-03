//go:build !linux && !darwin

package stream

import (
	"os"
)

func setWinsize(f *os.File, i *ResizeRequest) error {
	return nil
}
