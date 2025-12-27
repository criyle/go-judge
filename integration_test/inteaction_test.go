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

func TestInteraction_PipeStreaming(t *testing.T) {
	iterations := 10 // Run multiple times to catch race conditions
	t.Logf("Starting Interaction Pipe Test: %d iterations", iterations)

	for i := range iterations {
		t.Run(fmt.Sprintf("iter-%d", i), func(t *testing.T) {
			t.Parallel()

			// --- Program 1: The Producer ---
			// Writes 1MB of 'A's to stdout.
			// Using python for easy generation without compiling.
			producerScript := fmt.Sprintf("import sys; sys.stdout.write('A' * %d)", dataSize)
			producerCmd := Cmd{
				Args: []string{"python3", "-c", producerScript},
				Env:  []string{"PATH=/usr/bin:/bin"},
				Files: []*CmdFile{
					{Src: "/dev/null"},           // stdin (ignored)
					nil,                          // stdout (will be piped, but we can capture snippet)
					{Name: "stderr", Max: 10240}, // stderr
				},
				CPULimit:    2 * 1000 * 1000 * 1000,
				MemoryLimit: 64 * 1024 * 1024,
				ProcLimit:   1,
			}

			// --- Program 2: The Consumer ---
			// Reads from stdin and prints the length.
			consumerScript := "import sys; print(len(sys.stdin.read()))"
			consumerCmd := Cmd{
				Args: []string{"python3", "-c", consumerScript},
				Env:  []string{"PATH=/usr/bin:/bin"},
				Files: []*CmdFile{
					nil,                          // stdin (will be piped from producer)
					{Name: "stdout", Max: 10240}, // stdout (we read this to verify count)
					{Name: "stderr", Max: 10240}, // stderr
				},
				CPULimit:    2 * 1000 * 1000 * 1000,
				MemoryLimit: 64 * 1024 * 1024,
				ProcLimit:   1,
			}

			// --- Setup Pipe Mapping ---
			// Connect Producer(Idx 0).Stdout(1) -> Consumer(Idx 1).Stdin(0)
			mapping := []PipeMap{
				{
					In:  PipeIndex{Index: 0, Fd: 1},
					Out: PipeIndex{Index: 1, Fd: 0},
				},
			}

			reqBody := Request{
				Cmd:         []Cmd{producerCmd, consumerCmd},
				PipeMapping: mapping,
			}

			jsonBody, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			// --- Send Request ---
			client := &http.Client{Timeout: 5 * time.Second} // Strict timeout to catch deadlocks
			resp, err := client.Post(serverURL, "application/json", bytes.NewBuffer(jsonBody))
			if err != nil {
				t.Fatalf("API request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("API error %d: %s", resp.StatusCode, string(body))
			}

			// --- Verify Results ---
			var results []Result
			if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			if len(results) != 2 {
				t.Fatalf("expected 2 results, got %d", len(results))
			}

			prodRes := results[0]
			consRes := results[1]

			// Check 1: Producer Success
			if prodRes.Status != "Accepted" {
				t.Errorf("Producer failed: %s (Stderr: %s)", prodRes.Status, prodRes.Files["stderr"])
			}

			// Check 2: Consumer Success
			if consRes.Status != "Accepted" {
				t.Errorf("Consumer failed: %s (Stderr: %s)", consRes.Status, consRes.Files["stderr"])
			}

			// Check 3: Data Integrity
			// The consumer should have printed exactly "1048576" (plus newline)
			output := strings.TrimSpace(consRes.Files["stdout"])
			expected := fmt.Sprintf("%d", dataSize)

			if output != expected {
				t.Errorf("FAIL: Data loss in pipe.\nExpected bytes: %s\nActual bytes:   %s", expected, output)
				// If actual is 65536 (64KB), you have a deadlock/buffer freeze.
				if output == "65536" {
					t.Log("HINT: 65536 bytes indicates the producer filled the kernel pipe buffer and blocked because the consumer wasn't reading concurrently.")
				}
			}
		})
	}
}

func TestInteraction_Bidirectional_PingPong(t *testing.T) {
	iterations := 2
	t.Logf("Starting Bidirectional Ping-Pong Test: %d iterations", iterations)

	for i := range iterations {
		t.Run(fmt.Sprintf("iter-%d", i), func(t *testing.T) {
			t.Parallel()

			// --- Program A: The Verifier ---
			// 1. Generates 10000 random numbers.
			// 2. Writes one to stdout.
			// 3. FLUSHES (critical for interaction).
			// 4. Reads from stdin.
			// 5. Verifies equality.
			scriptA := `
import sys
import random

# Use unbuffered IO or flush explicitly
for i in range(80000):
    num = str(random.randint(1, 100000))
    
    # Send to B
    sys.stdout.write(num + '\n')
    sys.stdout.flush()
    
    # Read back from B
    reply = sys.stdin.readline().strip()
    
    if reply != num:
        sys.stderr.write(f"Mismatch at iter {i}: sent {num}, got {reply}\n")
        sys.exit(1)

sys.stderr.write("Program A: Finished successfully\n")
`

			// --- Program B: The Echo ---
			// 1. Reads line from stdin.
			// 2. Writes line to stdout.
			// 3. FLUSHES.
			scriptB := `
import sys

# Read until EOF
while True:
    line = sys.stdin.readline()
    if not line:
        break
        
    # Echo back to A
    sys.stdout.write(line)
    sys.stdout.flush()

sys.stderr.write("Program B: Finished successfully\n")
`

			// Construct Commands
			cmdA := Cmd{
				Args: []string{"python3", "-c", scriptA},
				Env:  []string{"PATH=/usr/bin:/bin"},
				Files: []*CmdFile{
					nil,                          // stdin (piped from B)
					nil,                          // stdout (piped to B)
					{Name: "stderr", Max: 10240}, // stderr (for debugging)
				},
				CPULimit:    5 * 1000 * 1000 * 1000, // 5s (interaction takes time)
				MemoryLimit: 64 * 1024 * 1024,
				ProcLimit:   1,
			}

			cmdB := Cmd{
				Args: []string{"python3", "-c", scriptB},
				Env:  []string{"PATH=/usr/bin:/bin"},
				Files: []*CmdFile{
					nil,                          // stdin (piped from A)
					nil,                          // stdout (piped to A)
					{Name: "stderr", Max: 10240}, // stderr
				},
				CPULimit:    5 * 1000 * 1000 * 1000,
				MemoryLimit: 64 * 1024 * 1024,
				ProcLimit:   1,
			}

			// --- Circular Pipe Mapping ---
			mapping := []PipeMap{
				// A(Stdout) -> B(Stdin)
				{
					In:  PipeIndex{Index: 0, Fd: 1}, // Cmd 0, Stdout
					Out: PipeIndex{Index: 1, Fd: 0}, // Cmd 1, Stdin
				},
				// B(Stdout) -> A(Stdin)
				{
					In:  PipeIndex{Index: 1, Fd: 1}, // Cmd 1, Stdout
					Out: PipeIndex{Index: 0, Fd: 0}, // Cmd 0, Stdin
				},
			}

			reqBody := Request{
				Cmd:         []Cmd{cmdA, cmdB},
				PipeMapping: mapping,
			}

			jsonBody, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			// Send Request
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Post(serverURL, "application/json", bytes.NewBuffer(jsonBody))
			if err != nil {
				t.Fatalf("API request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("API error %d: %s", resp.StatusCode, string(body))
			}

			// Verify Results
			var results []Result
			if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			if len(results) != 2 {
				t.Fatalf("expected 2 results, got %d", len(results))
			}

			// Verify Program A (The Verifier)
			// If A exits with 0 (Accepted), it means all 1000 checks passed.
			if results[0].Status != "Accepted" {
				t.Errorf("Program A (Verifier) Failed: %s\nStderr: %s",
					results[0].Status, results[0].Files["stderr"])
			}

			// Verify Program B (The Echo)
			// B might exit with "Accepted" (clean EOF) or "Runtime Error" depending on how
			// A closes the pipe. Ideally, when A finishes, it closes stdout, B gets EOF and exits clean.
			if results[1].Status != "Accepted" {
				// Note: It's acceptable for B to have SIGPIPE if A exits abruptly,
				// but in this script, A exits cleanly, so B should see EOF and exit cleanly.
				t.Logf("Program B (Echo) Status: %s\nStderr: %s",
					results[1].Status, results[1].Files["stderr"])
			}
		})
	}
}
