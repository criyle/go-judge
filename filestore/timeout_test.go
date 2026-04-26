package filestore

import "testing"

func TestTimeoutCloseIsIdempotent(t *testing.T) {
	fs := NewTimeout(NewFileLocalStore(t.TempDir()), 0, 1)
	if err := fs.Close(); err != nil {
		t.Fatalf("first close failed: %v", err)
	}
	if err := fs.Close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
}
