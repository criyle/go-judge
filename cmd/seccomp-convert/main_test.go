package main

import (
	"testing"

	"github.com/elastic/go-seccomp-bpf/arch"
	"golang.org/x/sys/unix"
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

func TestIoctlPolicyExcludesTerminalInjectionRequests(t *testing.T) {
	info, err := arch.GetInfo("x86_64")
	if err != nil {
		t.Fatal(err)
	}
	policy := generatePolicy([]group{{Names: []string{"ioctl"}, Action: "SCMP_ACT_ALLOW"}}, targetArchitectures[0], info)
	for _, generated := range policy {
		for _, name := range generated.names {
			if name == "ioctl" {
				t.Fatal("unrestricted ioctl allow rule was generated")
			}
		}
		if generated.key == "restricted-ioctl" {
			if len(generated.args) != 2 || generated.args[0].Index != 1 || generated.args[0].Value != unix.TIOCSTI || generated.args[0].Op != "SCMP_CMP_NE" || generated.args[1].Index != 1 || generated.args[1].Value != unix.TIOCLINUX || generated.args[1].Op != "SCMP_CMP_NE" {
				t.Fatalf("unexpected restricted ioctl conditions: %+v", generated.args)
			}
			return
		}
	}
	t.Fatal("restricted ioctl rule was not generated")
}

func TestSocketFamilyAllowlist(t *testing.T) {
	info, err := arch.GetInfo("x86_64")
	if err != nil {
		t.Fatal(err)
	}
	policy := generatePolicy(nil, targetArchitectures[0], info)
	want := map[uint64]bool{
		unix.AF_UNIX:  true,
		unix.AF_INET:  true,
		unix.AF_INET6: true,
	}
	got := make(map[uint64]bool)
	for _, generated := range policy {
		if generated.key != "socket-families" {
			continue
		}
		for _, condition := range generated.conditions {
			if condition.name != "socket" || len(condition.args) != 1 || condition.args[0].Index != 0 || condition.args[0].Op != "SCMP_CMP_EQ" {
				t.Fatalf("unexpected socket allow rule: %+v", generated)
			}
			got[condition.args[0].Value] = true
		}
	}
	if len(got) != len(want) {
		t.Fatalf("socket family allowlist = %v, want %v", got, want)
	}
	for family := range want {
		if !got[family] {
			t.Errorf("socket family %d missing from allowlist", family)
		}
	}
	if got[33] {
		t.Error("AF_RXRPC unexpectedly present in socket allowlist")
	}
}

func TestGetpgidOnlyAllowsCurrentProcess(t *testing.T) {
	info, err := arch.GetInfo("aarch64")
	if err != nil {
		t.Fatal(err)
	}
	policy := generatePolicy(nil, targetArchitectures[2], info)
	for _, generated := range policy {
		if generated.key != "getpgid-self" {
			continue
		}
		if len(generated.conditions) != 1 {
			t.Fatalf("getpgid conditions = %+v, want one self-only condition", generated.conditions)
		}
		condition := generated.conditions[0]
		if condition.name != "getpgid" || len(condition.args) != 1 || condition.args[0].Index != 0 || condition.args[0].Value != 0 || condition.args[0].Op != "SCMP_CMP_EQ" {
			t.Fatalf("getpgid self condition = %+v", condition)
		}
		return
	}
	t.Fatal("self-only getpgid rule was not generated")
}
