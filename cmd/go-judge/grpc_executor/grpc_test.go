package grpcexecutor

import (
	"testing"

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
