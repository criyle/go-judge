//go:build !linux && !darwin

package grpcexecutor

import (
	"os"

	"github.com/criyle/go-judge/pb"
)

func setWinsize(f *os.File, i *pb.StreamRequest_ExecResize) error {
	return nil
}
