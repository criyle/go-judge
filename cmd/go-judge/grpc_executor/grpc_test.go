package grpcexecutor

import (
	"testing"

	"github.com/criyle/go-judge/cmd/go-judge/stream"
	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/pb"
)

func TestConvertPBFileErrorType(t *testing.T) {
	tests := []struct {
		name string
		in   envexec.FileErrorType
		want pb.Response_FileError_ErrorType
	}{
		{name: "CopyInOpenFile", in: envexec.ErrCopyInOpenFile, want: pb.Response_FileError_CopyInOpenFile},
		{name: "CopyInCreateDir", in: envexec.ErrCopyInCreateDir, want: pb.Response_FileError_CopyInCreateFile},
		{name: "CopyInCreateFile", in: envexec.ErrCopyInCreateFile, want: pb.Response_FileError_CopyInCreateFile},
		{name: "CopyInCopyContent", in: envexec.ErrCopyInCopyContent, want: pb.Response_FileError_CopyInCopyContent},
		{name: "CopyOutOpen", in: envexec.ErrCopyOutOpen, want: pb.Response_FileError_CopyOutOpen},
		{name: "CopyOutNotRegularFile", in: envexec.ErrCopyOutNotRegularFile, want: pb.Response_FileError_CopyOutNotRegularFile},
		{name: "CopyOutSizeExceeded", in: envexec.ErrCopyOutSizeExceeded, want: pb.Response_FileError_CopyOutSizeExceeded},
		{name: "CopyOutCreateFile", in: envexec.ErrCopyOutCreateFile, want: pb.Response_FileError_CopyOutCreateFile},
		{name: "CopyOutCopyContent", in: envexec.ErrCopyOutCopyContent, want: pb.Response_FileError_CopyOutCopyContent},
		{name: "CollectSizeExceeded", in: envexec.ErrCollectSizeExceeded, want: pb.Response_FileError_CollectSizeExceeded},
		{name: "Symlink", in: envexec.ErrSymlink, want: pb.Response_FileError_Symlink},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := convertPBFileErrorType(tc.in); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestConvertPBStreamControl(t *testing.T) {
	request := pb.StreamRequest_builder{
		ExecControl: pb.StreamRequest_Control_builder{
			Index: 3,
			BeginTurn: pb.StreamRequest_Control_BeginTurn_builder{
				TurnId: 17, MoveCpuLimit: 200, TotalCpuLimit: 1000,
				WallLimit: 500, OutputFd: 1, Delimiter: []byte("\n"), MaxOutput: 4096,
			}.Build(),
		}.Build(),
	}.Build()
	control := request.GetExecControl()
	got := &stream.ControlRequest{Index: int(control.GetIndex())}
	begin := control.GetBeginTurn()
	got.BeginTurn = &stream.BeginTurnRequest{
		TurnID: begin.GetTurnId(), MoveCPULimit: begin.GetMoveCpuLimit(),
		TotalCPULimit: begin.GetTotalCpuLimit(), WallLimit: begin.GetWallLimit(),
		OutputFD: int(begin.GetOutputFd()), Delimiter: string(begin.GetDelimiter()),
		MaxOutput: int(begin.GetMaxOutput()),
	}
	if got.Index != 3 || got.BeginTurn.TurnID != 17 || got.BeginTurn.Delimiter != "\n" {
		t.Fatalf("unexpected control: %#v", got)
	}
}
