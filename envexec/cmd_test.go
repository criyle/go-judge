package envexec

import "testing"

func TestFileErrorTypeStringIncludesSymlink(t *testing.T) {
	if got := ErrSymlink.String(); got != "Symlink" {
		t.Fatalf("expected %q, got %q", "Symlink", got)
	}
}
