package filestore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileLocalStoreRejectsTraversalIDs(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileLocalStore(dir)

	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	if _, file := fs.Get("../../" + filepath.Base(outside)); file != nil {
		t.Fatalf("expected traversal id to be rejected")
	}
	if ok := fs.Remove("../../" + filepath.Base(outside)); ok {
		t.Fatalf("expected traversal id remove to fail")
	}
}

func TestFileLocalStoreAcceptsGeneratedLikeIDs(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileLocalStore(dir)

	id := "ABCDEFGH"
	path := filepath.Join(dir, id)
	if err := os.WriteFile(path, []byte("ok"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	name, file := fs.Get(id)
	if file == nil {
		t.Fatalf("expected file to be returned")
	}
	if name != id {
		t.Fatalf("unexpected name %q", name)
	}
}

func TestFileLocalStoreRemoveReportsDeleteFailure(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileLocalStore(dir)

	id := "ABCDEFGH"
	path := filepath.Join(dir, id)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("Mkdir error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "child"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	if ok := fs.Remove(id); ok {
		t.Fatalf("expected remove to report failure for non-empty directory")
	}
}
