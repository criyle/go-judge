package main

import (
	"testing"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/filestore"
)

type removeFailFileStore struct {
	filestore.FileStore
}

func (removeFailFileStore) Remove(string) bool { return false }

func TestMetricsFileStoreRemoveKeepsAccountingOnFailure(t *testing.T) {
	base := filestore.NewFileLocalStore(t.TempDir())
	store := newMetricsFileStore(removeFailFileStore{FileStore: base}).(*metricsFileStore)

	store.fileSize["file-id"] = 123
	if ok := store.Remove("file-id"); ok {
		t.Fatal("expected remove to fail")
	}
	if got := store.fileSize["file-id"]; got != 123 {
		t.Fatalf("expected size entry to be preserved, got %d", got)
	}
}

var _ envexec.File = (*envexec.FileInput)(nil)
