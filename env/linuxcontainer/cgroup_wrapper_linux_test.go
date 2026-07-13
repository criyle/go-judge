package linuxcontainer

import "testing"

func TestCgroupEventValue(t *testing.T) {
	content := []byte("populated 1\nfrozen 0\n")
	if got := cgroupEventValue(content, "frozen"); got != "0" {
		t.Fatalf("got %q", got)
	}
	if got := cgroupEventValue(content, "missing"); got != "" {
		t.Fatalf("got %q", got)
	}
}
