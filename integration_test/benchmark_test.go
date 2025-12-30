//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// Define the payload once to avoid overhead during the loop
var benchPayload []byte

func init() {
	// Pre-compute the JSON payload to measure HTTP throughput, not JSON marshaling speed
	req := Request{
		Cmd: []Cmd{{
			Args: []string{"/bin/true"}, // Fastest possible command
			Env:  []string{"PATH=/bin"},
			Files: []*CmdFile{
				{Content: "test"},
				{Name: "stdout", Max: 1024},
				{Name: "stderr", Max: 1024},
			},
			CPULimit:    1000 * 1000 * 1000,
			MemoryLimit: 64 * 1024 * 1024,
			ProcLimit:   50,
		}},
	}
	benchPayload, _ = json.Marshal(req)
}

func BenchmarkGoJudge_Concurrency(b *testing.B) {
	// We define the concurrency levels we want to test explicitly
	concurrencyLevels := []int{1, 2, 4, 8}

	for _, parallelism := range concurrencyLevels {
		b.Run(fmt.Sprintf("Parallel-%d", parallelism), func(b *testing.B) {
			client := &http.Client{
				Transport: &http.Transport{
					MaxIdleConns:        parallelism,
					MaxIdleConnsPerHost: parallelism,
					IdleConnTimeout:     30 * time.Second,
				},
				Timeout: 5 * time.Second,
			}

			b.SetParallelism(parallelism)
			b.ResetTimer() // Don't count setup time
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					resp, err := client.Post(serverURL, "application/json", bytes.NewReader(benchPayload))
					if err != nil {
						b.Errorf("Request failed: %v", err)
						continue
					}
					// Always read body to ensure connection reuse
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()

					if resp.StatusCode != 200 {
						b.Errorf("Status %d", resp.StatusCode)
					}
				}
			})
		})
	}
}
