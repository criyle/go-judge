//go:build integration && linux

package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSeccomp_DeniesRestrictedSyscalls(t *testing.T) {
	client := &http.Client{Timeout: 20 * time.Second}

	source := `#include <cerrno>
#include <cstdlib>
#include <iostream>
#include <linux/sched.h>
#include <sys/personality.h>
#include <sys/ptrace.h>
#include <sys/syscall.h>
#include <sys/uio.h>
#include <unistd.h>

static void check(const char* name, long result) {
    std::cout << name << "=" << (result == -1 ? errno : 0) << "\n";
}

int main() {
    errno = 0;
    check("ptrace", syscall(SYS_ptrace, PTRACE_TRACEME, 0, 0, 0));

    struct iovec empty = {nullptr, 0};
    errno = 0;
    check("process_vm_readv", syscall(SYS_process_vm_readv, getpid(), &empty, 0, &empty, 0, 0));
    errno = 0;
    check("process_vm_writev", syscall(SYS_process_vm_writev, getpid(), &empty, 0, &empty, 0, 0));

    errno = 0;
    check("personality", syscall(SYS_personality, 0));

    // Invalid arguments should reach the kernel. EPERM would indicate that
    // the inherited seccomp filter denied the syscall first.
    errno = 0;
    check("clone3", syscall(SYS_clone3, nullptr, 0));
    errno = 0;
    check("unshare", syscall(SYS_unshare, 0x80000000UL));

	return 0;
}`

	compileReq := Request{Cmd: []Cmd{{
		Args:     []string{"/usr/bin/g++", "-O2", "seccomp.cc", "-o", "seccomp", "-std=c++11"},
		Env:      []string{"PATH=/usr/bin:/bin"},
		Files:    []*CmdFile{{Src: "/dev/null"}, {Name: "stdout", Max: 10240}, {Name: "stderr", Max: 10240}},
		CPULimit: 3 * 1000 * 1000 * 1000, MemoryLimit: 256 * 1024 * 1024, ProcLimit: 50,
		CopyIn: map[string]CmdFile{"seccomp.cc": {Content: source}}, CopyOutCached: []string{"seccomp"},
	}}}
	compileResults := postSeccompRequest(t, client, compileReq)
	if compileResults[0].Status != "Accepted" {
		t.Fatalf("compile failed: %s; stderr: %s", compileResults[0].Status, compileResults[0].Files["stderr"])
	}
	executableID := compileResults[0].FileIDs["seccomp"]
	if executableID == "" {
		t.Fatal("compiler did not return a cached executable")
	}
	defer func() {
		req, _ := http.NewRequest(http.MethodDelete, fileURL+executableID, nil)
		resp, err := client.Do(req)
		if err == nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()

	runReq := Request{Cmd: []Cmd{{
		Args: []string{"./seccomp"}, Env: []string{"PATH=/usr/bin:/bin"},
		Files:    []*CmdFile{{Src: "/dev/null"}, {Name: "stdout", Max: 10240}, {Name: "stderr", Max: 10240}},
		CPULimit: 1 * 1000 * 1000 * 1000, MemoryLimit: 128 * 1024 * 1024, ProcLimit: 10,
		CopyIn: map[string]CmdFile{"seccomp": {FileID: executableID}},
	}}}
	results := postSeccompRequest(t, client, runReq)
	result := results[0]
	if result.Status != "Accepted" {
		t.Fatalf("probe failed: %s; stderr: %s", result.Status, result.Files["stderr"])
	}

	want := map[string]int{"ptrace": 1, "process_vm_readv": 1, "process_vm_writev": 1, "personality": 1}
	got := make(map[string]int)
	for _, line := range strings.Split(strings.TrimSpace(result.Files["stdout"]), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			t.Fatalf("malformed probe output %q", line)
		}
		errno, err := strconv.Atoi(parts[1])
		if err != nil {
			t.Fatalf("malformed errno in %q: %v", line, err)
		}
		got[parts[0]] = errno
	}
	for name, errno := range want {
		if got[name] != errno {
			t.Errorf("%s: got errno %d, want EPERM (1); output=%q", name, got[name], result.Files["stdout"])
		}
	}
	for _, name := range []string{"clone3", "unshare"} {
		if got[name] == 1 {
			t.Errorf("%s was denied by seccomp; output=%q", name, result.Files["stdout"])
		}
	}
}

func postSeccompRequest(t *testing.T, client *http.Client, request Request) []Result {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := client.Post(serverURL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("server returned %d", resp.StatusCode)
	}
	var results []Result
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	return results
}
