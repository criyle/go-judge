//go:build integration && linux

package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSeccomp_DeniesRestrictedSyscalls(t *testing.T) {
	client := &http.Client{Timeout: 20 * time.Second}

	source := `#include <cerrno>
#include <cstdlib>
#include <fstream>
#include <iostream>
#include <linux/sched.h>
#include <string>
#include <sys/personality.h>
#include <sys/ptrace.h>
#include <sys/socket.h>
#include <sys/syscall.h>
#include <sys/uio.h>
#include <unistd.h>

static void check(const char* name, long result) {
    std::cout << name << "=" << (result == -1 ? errno : 0) << "\n";
}

int main() {
    std::ifstream status("/proc/self/status");
    std::string statusLine;
    while (std::getline(status, statusLine)) {
        const std::string key = "Seccomp_filters:";
        if (statusLine.compare(0, key.size(), key) == 0) {
            std::cout << "seccomp_filters=" << std::stoi(statusLine.substr(key.size())) << "\n";
            break;
        }
    }

    errno = 0;
    check("ptrace", syscall(SYS_ptrace, PTRACE_TRACEME, 0, 0, 0));

    struct iovec empty = {nullptr, 0};
    errno = 0;
    check("process_vm_readv", syscall(SYS_process_vm_readv, getpid(), &empty, 0, &empty, 0, 0));
    errno = 0;
    check("process_vm_writev", syscall(SYS_process_vm_writev, getpid(), &empty, 0, &empty, 0, 0));

    errno = 0;
    check("personality", syscall(SYS_personality, 0));

    errno = 0;
    int unixSocket = socket(AF_UNIX, SOCK_STREAM, 0);
    check("socket_AF_UNIX", unixSocket);
    if (unixSocket >= 0) close(unixSocket);

    errno = 0;
    int ipv4Socket = socket(AF_INET, SOCK_STREAM, 0);
    check("socket_AF_INET", ipv4Socket);
    if (ipv4Socket >= 0) close(ipv4Socket);

    errno = 0;
    int ipv6Socket = socket(AF_INET6, SOCK_STREAM, 0);
    check("socket_AF_INET6", ipv6Socket);
    if (ipv6Socket >= 0) close(ipv6Socket);

    int pair[2];
    errno = 0;
    int pairResult = socketpair(AF_UNIX, SOCK_SEQPACKET, 0, pair);
    check("socketpair_AF_UNIX", pairResult);
    if (pairResult == 0) {
        close(pair[0]);
        close(pair[1]);
    }

    // AF_RXRPC is not part of the observed workload allowlist. Seccomp should
    // deny it before the kernel's RxRPC implementation handles the request.
    errno = 0;
    int rxrpcSocket = socket(33, SOCK_DGRAM, 0);
    check("socket_AF_RXRPC", rxrpcSocket);
    if (rxrpcSocket >= 0) close(rxrpcSocket);

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
	baselineFilters, err := hostSeccompFilterCount()
	if err != nil {
		t.Fatalf("read host seccomp filter count: %v", err)
	}

	want := map[string]int{
		"ptrace": 1, "process_vm_readv": 1, "process_vm_writev": 1, "personality": 1,
	}
	allowed := []string{"socket_AF_UNIX", "socket_AF_INET", "socketpair_AF_UNIX"}
	if hostCanCreateSocket(syscall.AF_INET6) {
		allowed = append(allowed, "socket_AF_INET6")
	} else {
		t.Log("host denies or lacks AF_INET6; skipping the AF_INET6 success assertion")
	}
	if hostCanCreateSocket(33) {
		want["socket_AF_RXRPC"] = 1
	} else {
		t.Log("host denies or lacks AF_RXRPC; generator allowlist is covered by a unit test")
	}
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
	for _, name := range allowed {
		if got[name] != 0 {
			t.Errorf("%s: got errno %d, want success; output=%q", name, got[name], result.Files["stdout"])
		}
	}
	if got["seccomp_filters"] <= baselineFilters {
		t.Errorf("sandbox has %d seccomp filters, want more than host baseline %d; output=%q", got["seccomp_filters"], baselineFilters, result.Files["stdout"])
	}
	for _, name := range []string{"clone3", "unshare"} {
		if got[name] == 1 {
			t.Errorf("%s was denied by seccomp; output=%q", name, result.Files["stdout"])
		}
	}
}

func TestSeccomp_ObservedSocketCompilers(t *testing.T) {
	client := &http.Client{Timeout: 30 * time.Second}
	workloads := []struct {
		name, sourceName, source, command string
		binaries                          []string
	}{
		{
			name:       "java",
			binaries:   []string{"/usr/bin/javac", "/usr/bin/java"},
			sourceName: "Main.java",
			source:     "public class Main { public static void main(String[] args) { System.out.println(3); } }\n",
			command:    "/usr/bin/javac Main.java && /usr/bin/java Main",
		},
		{
			name:       "rust",
			binaries:   []string{"/usr/bin/rustc"},
			sourceName: "main.rs",
			source:     "fn main() { println!(\"3\"); }\n",
			command:    "/usr/bin/rustc main.rs -o main && ./main",
		},
	}
	for _, workload := range workloads {
		t.Run(workload.name, func(t *testing.T) {
			for _, path := range workload.binaries {
				if _, err := os.Stat(path); err != nil {
					t.Skipf("%s is not present in the judge filesystem", path)
				}
			}
			request := Request{Cmd: []Cmd{{
				Args:     []string{"/bin/sh", "-c", workload.command},
				Env:      []string{"PATH=/usr/bin:/bin"},
				Files:    []*CmdFile{{Src: "/dev/null"}, {Name: "stdout", Max: 1024}, {Name: "stderr", Max: 1024}},
				CPULimit: 10 * 1000 * 1000 * 1000, MemoryLimit: 512 * 1024 * 1024, ProcLimit: 50,
				CopyIn: map[string]CmdFile{workload.sourceName: {Content: workload.source}},
			}}}
			results := postSeccompRequest(t, client, request)
			if results[0].Status != "Accepted" {
				t.Fatalf("%s compiler/runtime failed under seccomp: %s; stdout=%q stderr=%q", workload.name, results[0].Status, results[0].Files["stdout"], results[0].Files["stderr"])
			}
			if got := strings.TrimSpace(results[0].Files["stdout"]); got != "3" {
				t.Fatalf("%s output = %q, want %q", workload.name, got, "3")
			}
		})
	}
}

func hostCanCreateSocket(family int) bool {
	fd, err := syscall.Socket(family, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return false
	}
	_ = syscall.Close(fd)
	return true
}

func hostSeccompFilterCount() (int, error) {
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(status), "\n") {
		if strings.HasPrefix(line, "Seccomp_filters:") {
			return strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Seccomp_filters:")))
		}
	}
	return 0, fmt.Errorf("Seccomp_filters is missing from /proc/self/status")
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
