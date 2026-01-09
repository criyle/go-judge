//go:build integration

package integration_test

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fileSize = 1024 * 1024 // 1MB
// Max size to generate for the pool (2MB)
const maxPoolSize = 20 * 1024 * 1024

// Base size for the test (1MB)
const baseSize = 1024 * 1024

func TestRaceCondition_CopyInLargeFile_Src(t *testing.T) {
	// 1. Generate a large pool of random data ONCE.
	// We will slice this differently for each test to get unique sizes without
	// the overhead of reading from /dev/urandom 50 times.
	dataPool := make([]byte, maxPoolSize)
	if _, err := rand.Read(dataPool); err != nil {
		t.Fatalf("failed to generate random data pool: %v", err)
	}

	iterations := 50
	t.Logf("Starting reproduction test: %d iterations, %d bytes file using 'src' path", iterations, fileSize)

	for i := range iterations {
		t.Run(fmt.Sprintf("iter-%d", i), func(t *testing.T) {
			t.Parallel()

			// 2. Calculate a unique size for this iteration
			// Formula: 1MB + (iteration * 10KB) + specific offset to test odd alignments
			// e.g., 1048576, 1058817, 1069058...
			// This ensures we hit different buffer boundaries.
			offset := i * 10240
			oddByte := i * 1 // Add a single byte shift to avoid perfect alignment
			currentSize := baseSize + offset + oddByte

			// Safety check to ensure we don't go out of bounds of our pool
			if currentSize > maxPoolSize {
				t.Fatalf("Test configuration error: calculated size %d exceeds pool %d", currentSize, maxPoolSize)
			}

			// Slice the data for this specific test case
			content := dataPool[:currentSize]

			// A. Create the source file on the host
			subDir := t.TempDir()
			fileName := fmt.Sprintf("test_file_%d.bin", i)
			hostFilePath := filepath.Join(subDir, fileName)

			if err := os.WriteFile(hostFilePath, content, 0644); err != nil {
				t.Fatalf("failed to write host temp file: %v", err)
			}
			defer os.Remove(hostFilePath) // Clean up individual file

			// B. Construct the request
			cmd := Cmd{
				Args: []string{"/bin/sh", "-c", "wc -c < container_input.bin"},
				Env:  []string{"PATH=/bin:/usr/bin"},

				// CRITICAL: Define stdin (empty), stdout, and stderr to capture output
				Files: []*CmdFile{
					{Src: "/dev/null"},           // stdin
					{Name: "stdout", Max: 10240}, // stdout
					{Name: "stderr", Max: 10240}, // stderr
				},

				CopyIn: map[string]CmdFile{
					"container_input.bin": {Src: hostFilePath},
				},

				CPULimit:    2 * 1000 * 1000 * 1000, // 2s
				MemoryLimit: 128 * 1024 * 1024,      // 128MB
				ProcLimit:   50,
			}

			reqBody := Request{Cmd: []Cmd{cmd}}
			jsonBody, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			// C. Send to go-judge
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Post(serverURL, "application/json", bytes.NewBuffer(jsonBody))
			if err != nil {
				t.Fatalf("API request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("API error %d: %s", resp.StatusCode, string(body))
			}

			// D. Verify Result
			var results []Result
			if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
				t.Fatalf("decode response failed: %v", err)
			}

			if len(results) == 0 {
				t.Fatal("empty results")
			}
			res := results[0]

			// Check 1: Status
			if res.Status != "Accepted" {
				t.Errorf("FAIL: Status is %s. Error: %s, Stderr: %s",
					res.Status, res.Error, res.Files["stderr"])
				t.FailNow()
			}

			// Check 2: Size verification from stdout
			stdout := strings.TrimSpace(res.Files["stdout"])
			expected := fmt.Sprintf("%d", currentSize)

			if stdout != expected {
				t.Errorf("FAIL: Data corruption detected.\nExpected: %s\nActual:   %s",
					expected, stdout)
				t.FailNow()
			}
		})
	}
}

func TestRaceCondition_CopyInAndExec_Src(t *testing.T) {
	// Define the number of parallel attempts
	// 10000 attempt could reproduce the ETXTBSY issue
	iterations := 10000
	t.Logf("Starting Exec Race Test: %d iterations", iterations)

	// 1. Locate a suitable binary on the Host to copy
	// /bin/echo is perfect: small, static-ish, and verifies arguments
	binaryPath := "/bin/echo"
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		// Fallback for some systems (like Mac/Docker)
		binaryPath = "/usr/bin/echo"
	}

	// Read the binary content once into memory
	binaryContent, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("failed to read host binary %s: %v", binaryPath, err)
	}

	t.Logf("Starting Binary Race Test: %d iterations using %s (%d bytes)",
		iterations, binaryPath, len(binaryContent))

	for i := range iterations {
		t.Run(fmt.Sprintf("iter-%d", i), func(t *testing.T) {
			t.Parallel()

			// 1. Prepare Unique Executable Content
			// We use a shell script with a unique token to verify the *content* // was fully flushed before execution started.
			token := fmt.Sprintf("TOKEN_EXEC_%d_%d", time.Now().UnixNano(), i)

			// 2. Create the Source File on Host
			// We give it 0755 permissions. go-judge should copy these permissions
			// or at least allow us to execute it if configured correctly.
			tmpDir := t.TempDir()
			hostSrcPath := filepath.Join(tmpDir, "echo_copy")
			if err := os.WriteFile(hostSrcPath, binaryContent, 0755); err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			// Note: We don't defer remove here because t.TempDir cleans up automatically
			// and we are inside a parallel subtest.

			// 3. Construct the Request
			cmd := Cmd{
				// Execute the file directly to trigger kernel execve checks
				Args: []string{"./my_echo", token},
				Env:  []string{"PATH=/bin:/usr/bin"},

				Files: []*CmdFile{
					{Src: "/dev/null"},           // stdin
					{Name: "stdout", Max: 10240}, // stdout
					{Name: "stderr", Max: 10240}, // stderr
				},

				// Copy the host file to "./runner" inside the container
				CopyIn: map[string]CmdFile{
					"my_echo": {Src: hostSrcPath},
				},

				CPULimit:    1 * 1000 * 1000 * 1000, // 1s
				MemoryLimit: 64 * 1024 * 1024,       // 64MB
				ProcLimit:   50,
			}

			reqBody := Request{Cmd: []Cmd{cmd}}
			jsonBody, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			// 4. Send Request
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Post(serverURL, "application/json", bytes.NewBuffer(jsonBody))
			if err != nil {
				t.Fatalf("API request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("API error %d: %s", resp.StatusCode, string(body))
			}

			// 5. Verify Result
			var results []SanityResult
			if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
				t.Fatalf("decode response failed: %v", err)
			}

			if len(results) == 0 {
				t.Fatal("empty results")
			}
			res := results[0]

			// --- VERIFICATION ---

			// A. Check for "File Error" (ETXTBUSY often shows up here or in Status)
			if res.Status != "Accepted" {
				t.Errorf("FAIL: Status is %s.\nError:  %s\nStderr: %s",
					res.Status, res.Error, res.Files["stderr"])
				t.FailNow()
			}

			// B. Check Stdout for Data Corruption
			// If the file was run before write flush, stdout might be empty.
			stdout := strings.TrimSpace(res.Files["stdout"])
			if stdout != token {
				t.Errorf("FAIL: Output mismatch (Race detected?).\nExpected: %s\nActual:   %s",
					token, stdout)
				t.FailNow()
			}
		})
	}
}
