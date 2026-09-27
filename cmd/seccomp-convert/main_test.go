package main

import (
	"testing"

	"github.com/elastic/go-seccomp-bpf/arch"
)

func TestStrictSyscallsAreExcluded(t *testing.T) {
	info, err := arch.GetInfo("x86_64")
	if err != nil {
		t.Fatal(err)
	}
	names := validNames([]string{
		"read",
		"personality",
		"process_vm_readv",
		"process_vm_writev",
		"ptrace",
	}, info)
	if len(names) != 1 || names[0] != "read" {
		t.Fatalf("unexpected allowed names: %v", names)
	}
}
