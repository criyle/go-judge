//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestInteraction_Bidirectional_ProxyMatrix(t *testing.T) {
	const iterations = 100000

	scriptA := fmt.Sprintf(`
import sys

sys.stdout.reconfigure(line_buffering=True)

sys.stdout.write("PING\n")
sys.stdout.flush()
reply = sys.stdin.readline()
if not reply:
    sys.stderr.write("peer disconnected immediately\n")
    sys.exit(1)

for i in range(%d):
    sys.stdout.write(f"{i}\n")
    reply = sys.stdin.readline().strip()
    if reply != str(i):
        sys.stderr.write(f"mismatch at iter {i}: got {reply}\n")
        sys.exit(1)
`, iterations)

	scriptB := `
import sys

sys.stdout.reconfigure(line_buffering=True)

while True:
    line = sys.stdin.readline()
    if not line:
        break
    sys.stdout.write(line)
`

	type placement struct {
		name            string
		aCPU            string
		bCPU            string
		expectWorkerCPU bool
	}
	type mode struct {
		name            string
		proxy           bool
		disableZeroCopy bool
	}

	placements := []placement{
		{name: "worker-allocated-same-cpu", expectWorkerCPU: true},
		{name: "split-cpu-0-1", aCPU: "0", bCPU: "1"},
	}
	modes := []mode{
		{name: "none"},
		{name: "std", proxy: true, disableZeroCopy: true},
		{name: "splice", proxy: true, disableZeroCopy: false},
	}

	for _, placement := range placements {
		for _, mode := range modes {
			name := fmt.Sprintf("%s/%s", mode.name, placement.name)
			t.Run(name, func(t *testing.T) {
				reqBody := Request{
					Cmd: []Cmd{
						{
							Args: []string{"python3", "-c", scriptA},
							Env:  []string{"PATH=/usr/bin:/bin"},
							Files: []*CmdFile{
								nil,
								nil,
								{Name: "stderr", Max: 10240},
							},
							CPULimit:    20 * 1000 * 1000 * 1000,
							MemoryLimit: 64 * 1024 * 1024,
							ProcLimit:   1,
							CPUSetLimit: placement.aCPU,
						},
						{
							Args: []string{"python3", "-c", scriptB},
							Env:  []string{"PATH=/usr/bin:/bin"},
							Files: []*CmdFile{
								nil,
								nil,
								{Name: "stderr", Max: 10240},
							},
							CPULimit:    20 * 1000 * 1000 * 1000,
							MemoryLimit: 64 * 1024 * 1024,
							ProcLimit:   1,
							CPUSetLimit: placement.bCPU,
						},
					},
					PipeMapping: []PipeMap{
						{
							In:              PipeIndex{Index: 0, Fd: 1},
							Out:             PipeIndex{Index: 1, Fd: 0},
							Proxy:           mode.proxy,
							DisableZeroCopy: mode.disableZeroCopy,
						},
						{
							In:              PipeIndex{Index: 1, Fd: 1},
							Out:             PipeIndex{Index: 0, Fd: 0},
							Proxy:           mode.proxy,
							DisableZeroCopy: mode.disableZeroCopy,
						},
					},
				}

				results, wallTime := postRunRequest(t, reqBody, 45*time.Second)
				if len(results) != 2 {
					t.Fatalf("expected 2 results, got %d", len(results))
				}
				for i, result := range results {
					if result.Status != "Accepted" {
						t.Fatalf("cmd %d failed: status=%s stderr=%s error=%s", i, result.Status, result.Files["stderr"], result.Error)
					}
				}

				t.Logf(
					"case=%s wall=%v cmd0_cpu=%s cmd1_cpu=%s relay_cpu=%s result0_time=%v result0_runtime=%v result1_time=%v result1_runtime=%v",
					name,
					wallTime,
					orDefault(placement.aCPU, "<worker>"),
					orDefault(placement.bCPU, "<worker>"),
					relayCPULabel(placement.expectWorkerCPU),
					time.Duration(results[0].Time),
					time.Duration(results[0].RunTime),
					time.Duration(results[1].Time),
					time.Duration(results[1].RunTime),
				)
			})
		}
	}
}

func postRunRequest(t *testing.T, reqBody Request, timeout time.Duration) ([]Result, time.Duration) {
	t.Helper()

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	client := &http.Client{Timeout: timeout}
	start := time.Now()
	resp, err := client.Post(serverURL, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		t.Fatalf("API request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("API error %d: %s", resp.StatusCode, string(body))
	}

	var results []Result
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	return results, time.Since(start)
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func relayCPULabel(expectWorkerCPU bool) string {
	if expectWorkerCPU {
		return "<worker>"
	}
	return "<worker>"
}
