package seccomp

import _ "embed"

// DefaultProfile contains the built-in architecture-aware seccomp policy.
// External profiles can be selected by passing the -seccomp flag.
//
//go:embed moby.yaml
var DefaultProfile []byte
