//go:build linux || darwin

package stream

import (
	"os"

	"github.com/creack/pty"
)

func setWinsize(f *os.File, i *ResizeRequest) error {
	winSize := &pty.Winsize{
		Rows: uint16(i.Rows),
		Cols: uint16(i.Cols),
		X:    uint16(i.X),
		Y:    uint16(i.Y),
	}
	return pty.Setsize(f, winSize)
}
