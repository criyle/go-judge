package env

import (
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestEmbeddedDefaultSeccompProfileBuilds(t *testing.T) {
	filter, err := readSeccompConf("")
	if err != nil {
		t.Fatalf("read embedded seccomp profile: %v", err)
	}
	if len(filter) == 0 {
		t.Fatal("embedded seccomp profile produced an empty filter")
	}
}

func TestSeccompCanBeForceDisabled(t *testing.T) {
	filter, err := prepareSeccomp(Config{NoSeccomp: true}, zap.NewNop())
	if err != nil {
		t.Fatalf("force-disabled seccomp returned an error: %v", err)
	}
	if filter != nil {
		t.Fatal("force-disabled seccomp returned a filter")
	}
}

func TestDefaultSeccompProfileBuilds(t *testing.T) {
	filter, err := readSeccompConf(filepath.Join("..", "seccomp", "moby.yaml"))
	if err != nil {
		t.Fatalf("read default seccomp profile: %v", err)
	}
	if len(filter) == 0 {
		t.Fatal("default seccomp profile produced an empty filter")
	}
}

func TestMissingSeccompOverrideFails(t *testing.T) {
	if _, err := readSeccompConf(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("missing seccomp override unexpectedly succeeded")
	}
}
