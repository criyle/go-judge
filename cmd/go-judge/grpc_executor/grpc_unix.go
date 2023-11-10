//go:build linux || darwin

package grpcexecutor

import (
	"os"

	"github.com/creack/pty"
	"github.com/criyle/go-judge/pb"
)

func setWinsize(f *os.File, i *pb.StreamRequest_ExecResize) error {
	winSize := &pty.Winsize{
		Rows: uint16(i.ExecResize.Rows),
		Cols: uint16(i.ExecResize.Cols),
		X:    uint16(i.ExecResize.X),
		Y:    uint16(i.ExecResize.Y),
	}
	return pty.Setsize(f, winSize)
}
