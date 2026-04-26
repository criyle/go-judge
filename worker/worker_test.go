package worker

import (
	"errors"
	"os"
	"testing"

	"github.com/criyle/go-judge/envexec"
)

type failingFileStore struct{}

func (failingFileStore) Add(name, path string) (string, error) { return "", errors.New("add failed") }
func (failingFileStore) Remove(string) bool                    { return false }
func (failingFileStore) Get(string) (string, envexec.File)     { return "", nil }
func (failingFileStore) List() map[string]string               { return nil }
func (failingFileStore) New() (*os.File, error)                { return nil, errors.New("not implemented") }
func (failingFileStore) Close() error                          { return nil }

func TestConvertResultClosesCachedFileOnAddFailure(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "cached-out-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}

	w := &worker{fs: failingFileStore{}}
	res := w.convertResult(envexec.Result{
		Files: map[string]*os.File{"out": tmp},
	}, Cmd{
		CopyOutCached: []CmdCopyOutFile{{Name: "out"}},
	})

	if res.Status != envexec.StatusFileError {
		t.Fatalf("expected status %v, got %v", envexec.StatusFileError, res.Status)
	}
	if res.Error != "add failed" {
		t.Fatalf("expected add failure, got %q", res.Error)
	}
	if _, err := tmp.Stat(); err == nil {
		t.Fatal("expected cached file to be closed on add failure")
	}
}
