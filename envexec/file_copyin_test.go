package envexec

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"golang.org/x/sync/errgroup"
)

type stubEnvironment struct {
	openFunc    func([]OpenParam) ([]OpenResult, error)
	symlinkFunc func([]SymlinkParam) ([]error, error)
}

func (s stubEnvironment) Execve(context.Context, ExecveParam) (Process, error) {
	return nil, errors.New("not implemented")
}

func (s stubEnvironment) Open(params []OpenParam) ([]OpenResult, error) {
	if s.openFunc != nil {
		return s.openFunc(params)
	}
	return nil, nil
}

func (s stubEnvironment) Symlink(params []SymlinkParam) ([]error, error) {
	if s.symlinkFunc != nil {
		return s.symlinkFunc(params)
	}
	return nil, nil
}

func TestCopyInReturnsPerFileErrorsOnBatchOpenFailure(t *testing.T) {
	env := stubEnvironment{
		openFunc: func([]OpenParam) ([]OpenResult, error) {
			return nil, errors.New("open failed")
		},
	}

	fileErrors, err := copyIn(env, map[string]File{
		"one.txt": NewFileReader(strings.NewReader("one")),
		"two.txt": NewFileReader(strings.NewReader("two")),
	})
	if err == nil {
		t.Fatal("expected batch open error")
	}
	if len(fileErrors) != 2 {
		t.Fatalf("expected 2 file errors, got %d", len(fileErrors))
	}
	for _, fe := range fileErrors {
		if fe.Type != ErrCopyInCreateFile {
			t.Fatalf("expected create file error, got %v", fe.Type)
		}
		if fe.Message != "open failed" {
			t.Fatalf("expected error message %q, got %q", "open failed", fe.Message)
		}
	}
}

func TestCopyOutFilesSkipsEmptyBatch(t *testing.T) {
	env := stubEnvironment{
		openFunc: func([]OpenParam) ([]OpenResult, error) {
			t.Fatal("Open should not be called for empty copyOut")
			return nil, nil
		},
	}

	cmd := &Cmd{}
	gotFiles := map[string]*os.File{}
	err := copyOutFiles(new(errgroup.Group), env, cmd, nil, func(f *os.File, name string) {
		gotFiles[name] = f
	}, func(FileError) {
		t.Fatal("addError should not be called for empty copyOut")
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(gotFiles) != 0 {
		t.Fatalf("expected no files, got %d", len(gotFiles))
	}
}
