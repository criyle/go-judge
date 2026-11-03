//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	swapTestMemoryLimit = 32 * 1024 * 1024
	swapTestAllocation  = 256 * 1024 * 1024
)

func TestMemoryLimitDoesNotPermitSwapEscape(t *testing.T) {
	memoryCgroupEnabled, memorySwapLimitEnabled := serverCgroupMemoryConfig(t)
	if !memoryCgroupEnabled {
		t.Skip("go-judge server is not using a memory cgroup")
	}
	if !memorySwapLimitEnabled {
		t.Skip("go-judge server did not enable cgroup swap limits")
	}
	if !swapControllerEnabled(t) {
		t.Skip("kernel does not expose swap accounting in this process's cgroup (v1 memory.memsw or v2 memory.swap)")
	}
	if free := hostSwapFree(t); free < swapTestAllocation {
		t.Skipf("requires at least %d MiB free host swap; have %d MiB", swapTestAllocation>>20, free>>20)
	}

	result := runMemorySwapProbe(t, swapTestAllocation)
	if result.Status != "Memory Limit Exceeded" {
		t.Fatalf("expected memory limit exceeded, got status=%q (exit=%d, stdout=%q, stderr=%q)", result.Status, result.ExitStatus, result.Files["stdout"], result.Files["stderr"])
	}
	if result.ExitStatus == 0 && strings.Contains(result.Files["stdout"], "ALLOCATION_COMPLETED") {
		t.Fatalf("workload completed despite %d MiB memory limit by using host swap (status=%q, reported memory=%d bytes)", swapTestMemoryLimit>>20, result.Status, result.Memory)
	}
	if result.ExitStatus == 0 {
		t.Fatalf("expected memory limit to terminate workload, got exit status 0 (status=%q, stdout=%q)", result.Status, result.Files["stdout"])
	}
	if strings.Contains(result.Files["stdout"], "ALLOCATION_COMPLETED") {
		t.Fatalf("workload printed its completion marker before being terminated: %q", result.Files["stdout"])
	}
}

func serverCgroupMemoryConfig(t *testing.T) (memoryCgroupEnabled, memorySwapLimitEnabled bool) {
	t.Helper()
	resp, err := http.Get(configURL)
	if err != nil {
		t.Fatalf("fetch server config: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("config endpoint returned HTTP %d", resp.StatusCode)
	}
	var config struct {
		RunnerConfig struct {
			CgroupType             int      `json:"cgroupType"`
			CgroupControllers      []string `json:"cgroupControllers"`
			MemorySwapLimitEnabled bool     `json:"memorySwapLimitEnabled"`
		} `json:"runnerConfig"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		t.Fatalf("decode server config: %v", err)
	}
	for _, controller := range config.RunnerConfig.CgroupControllers {
		if controller == "memory" && config.RunnerConfig.CgroupType != 0 {
			memoryCgroupEnabled = true
			break
		}
	}
	return memoryCgroupEnabled, config.RunnerConfig.MemorySwapLimitEnabled
}

func runMemorySwapProbe(t *testing.T, allocation uint64) Result {
	t.Helper()
	// Touch every page so this is real anonymous-memory use, rather than a
	// mostly uncommitted virtual allocation. The marker proves completion.
	program := fmt.Sprintf("n=%d; x=bytearray(n); x[::4096]=b'x'*(n//4096); print('ALLOCATION_COMPLETED')", allocation)
	request := Request{Cmd: []Cmd{{
		Args:         []string{"/usr/bin/python3", "-c", program},
		Env:          []string{"PATH=/usr/bin:/bin"},
		Files:        []*CmdFile{{Src: "/dev/null"}, {Name: "stdout", Max: 1024}, {Name: "stderr", Max: 1024}},
		CPULimit:     10_000_000_000,
		RealCPULimit: 15_000_000_000,
		MemoryLimit:  swapTestMemoryLimit,
		ProcLimit:    10,
	}}}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(serverURL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run endpoint returned HTTP %d", resp.StatusCode)
	}

	var results []Result
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		t.Fatalf("decode run result: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one command result, got %d", len(results))
	}
	return results[0]
}

func swapControllerEnabled(t *testing.T) bool {
	t.Helper()
	cgroupData, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		t.Skipf("cannot inspect process cgroup: %v", err)
	}
	mountData, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		t.Skipf("cannot inspect cgroup mounts: %v", err)
	}

	for _, cgroupLine := range strings.Split(string(cgroupData), "\n") {
		parts := strings.SplitN(cgroupLine, ":", 3)
		if len(parts) != 3 {
			continue
		}
		isV2 := parts[0] == "0" && parts[1] == ""
		isV1Memory := containsCSV(parts[1], "memory")
		if !isV2 && !isV1Memory {
			continue
		}

		for _, mountLine := range strings.Split(string(mountData), "\n") {
			fields := strings.Fields(mountLine)
			separator := -1
			for i, field := range fields {
				if field == "-" {
					separator = i
					break
				}
			}
			if separator < 0 || separator+3 >= len(fields) || len(fields) < 6 {
				continue
			}
			fsType := fields[separator+1]
			if (isV2 && fsType != "cgroup2") || (isV1Memory && (fsType != "cgroup" || !containsCSV(fields[separator+3], "memory"))) {
				continue
			}

			mountRoot := filepath.Clean(unescapeMountInfo(fields[3]))
			cgroupPath := filepath.Clean(parts[2])
			var relative string
			if cgroupPath == mountRoot {
				relative = "."
			} else if strings.HasPrefix(cgroupPath, strings.TrimSuffix(mountRoot, "/")+"/") {
				relative = strings.TrimPrefix(cgroupPath, strings.TrimSuffix(mountRoot, "/")+"/")
			} else {
				continue
			}

			controlFile := "memory.swap.max"
			if isV1Memory {
				controlFile = "memory.memsw.limit_in_bytes"
			}
			controlPath := filepath.Join(unescapeMountInfo(fields[4]), relative, controlFile)
			if _, err := os.Stat(controlPath); err == nil {
				return true
			}
		}
	}
	return false
}

func containsCSV(value, item string) bool {
	for _, field := range strings.Split(value, ",") {
		if field == item {
			return true
		}
	}
	return false
}

func unescapeMountInfo(value string) string {
	return strings.NewReplacer("\\040", " ", "\\011", "\t", "\\012", "\n", "\\134", "\\").Replace(value)
}

func hostSwapFree(t *testing.T) uint64 {
	t.Helper()
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		t.Skipf("cannot inspect host swap availability: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "SwapFree:") {
			var kib uint64
			if _, err := fmt.Sscanf(line, "SwapFree: %d kB", &kib); err != nil {
				t.Skipf("cannot parse host SwapFree: %v", err)
			}
			return kib * 1024
		}
	}
	t.Skip("host does not report SwapFree")
	return 0
}
