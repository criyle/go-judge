//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSanity_BasicFunctionality(t *testing.T) {
	// 1. Define the Expected Behavior
	type Expectation struct {
		Status        string
		ErrorContains string            // Substring check for the error message
		FilesContains map[string]string // Check if stdout/stderr contains specific text
		FileIDs       map[string]string // Check if cached file ids are present
	}

	// 2. Define the Test Case Structure
	type TestCase struct {
		Name   string
		Input  Cmd
		Expect Expectation
	}

	// 3. Define the Default Configuration (Base Template)
	baseCmd := Cmd{
		Env: []string{"PATH=/usr/bin:/bin"},
		Files: []*CmdFile{
			{Src: "/dev/null"},           // stdin
			{Name: "stdout", Max: 10240}, // stdout
			{Name: "stderr", Max: 10240}, // stderr
		},
		CPULimit:     10 * 1000 * 1000 * 1000, // 10s
		RealCPULimit: 12 * 1000 * 1000 * 1000,
		MemoryLimit:  100 * 1024 * 1024, // 100MB
		ProcLimit:    50,
	}

	// 4. THE TABLE
	tests := []TestCase{
		{
			Name: "Basic Hello World",
			Input: Cmd{
				Args: []string{"/bin/echo", "hello world"},
			},
			Expect: Expectation{
				Status: "Accepted",
				FilesContains: map[string]string{
					"stdout": "hello world",
				},
			},
		},
		{
			Name: "Symlink /dev/urandom CopyOut Failure",
			// This matches your specific request
			Input: Cmd{
				Args:    []string{"/bin/ln", "-s", "/dev/urandom", "out"},
				CopyOut: []string{"out"},
			},
			Expect: Expectation{
				// Depending on implementation, this usually returns "File Error"
				// or the specific error in the Error field
				Status: "File Error",
			},
		},
		{
			Name: "Time Limit Exceeded",
			Input: Cmd{
				Args:         []string{"/bin/sleep", "2"},
				CPULimit:     1 * 1000 * 1000 * 1000,
				RealCPULimit: 1 * 1000 * 1000 * 1000,
			},
			Expect: Expectation{
				Status: "Time Limit Exceeded",
			},
		},
		{
			Name: "Environment Variable Check",
			Input: Cmd{
				Args: []string{"/bin/sh", "-c", "echo $MY_VAR"},
				Env:  []string{"PATH=/bin", "MY_VAR=integration_test"},
			},
			Expect: Expectation{
				Status: "Accepted",
				FilesContains: map[string]string{
					"stdout": "integration_test",
				},
			},
		},
		{
			Name: "Signal Check",
			Input: Cmd{
				Args: []string{"/bin/bash", "-c", "kill -SIGINT 1"},
			},
			Expect: Expectation{
				Status: "Accepted",
			},
		},
		{
			Name: "Copy in Sub Directory",
			Input: Cmd{
				Args: []string{"/bin/ls", "test_dir"},
				CopyIn: map[string]CmdFile{
					"test_dir/test_file": {Content: "content"},
				},
			},
			Expect: Expectation{
				Status: "Accepted",
				FilesContains: map[string]string{
					"stdout": "test_file",
				},
			},
		},
		{
			Name: "Copy in Temp Directory",
			Input: Cmd{
				Args: []string{"/bin/ls", "/tmp"},
				CopyIn: map[string]CmdFile{
					"/tmp/test_file": {Content: "content"},
				},
			},
			Expect: Expectation{
				Status: "Accepted",
				FilesContains: map[string]string{
					"stdout": "test_file",
				},
			},
		},
		{
			Name: "Copy out File Error",
			Input: Cmd{
				Args:    []string{"/bin/ls"},
				CopyOut: []string{"test"},
			},
			Expect: Expectation{
				Status: "File Error",
			},
		},
		{
			Name: "Copy out Optional File Accepted",
			Input: Cmd{
				Args:    []string{"/bin/ls"},
				CopyOut: []string{"test?"},
			},
			Expect: Expectation{
				Status: "Accepted",
			},
		},
		{
			Name: "Copy out Cached File",
			Input: Cmd{
				Args:          []string{"/bin/sh", "-c", "printf hello > out && cat out"},
				CopyOutCached: []string{"out"},
			},
			Expect: Expectation{
				Status: "Accepted",
				FilesContains: map[string]string{
					"stdout": "hello",
				},
				FileIDs: map[string]string{
					"out": "",
				},
			},
		},
		{
			Name: "Stack Limit",
			Input: Cmd{
				Args:       []string{"/bin/bash", "-c", "ulimit -s"},
				StackLimit: 102400000,
			},
			Expect: Expectation{
				Status: "Accepted",
				FilesContains: map[string]string{
					"stdout": "100000",
				},
			},
		},
		{
			Name: "No Symlink Escape",
			Input: Cmd{
				Args:    []string{"/bin/ln", "-s", "/etc/passwd", "out.txt"},
				CopyOut: []string{"out.txt"},
			},
			Expect: Expectation{
				Status: "File Error",
			},
		},
		{
			Name: "Copy out max",
			Input: Cmd{
				Args: []string{"/bin/cat", "input.txt"},
				CopyIn: map[string]CmdFile{
					"input.txt": {Content: "1234567890"},
				},
				CopyOut:    []string{"input.txt"},
				CopyOutMax: 5,
			},
			Expect: Expectation{
				Status: "File Error",
			},
		},
		{
			Name: "Copy out Truncate",
			Input: Cmd{
				Args: []string{"/bin/cat", "input.txt"},
				CopyIn: map[string]CmdFile{
					"input.txt": {Content: "1234567890"},
				},
				CopyOut:         []string{"input.txt"},
				CopyOutMax:      5,
				CopyOutTruncate: true,
			},
			Expect: Expectation{
				Status: "File Error",
				FilesContains: map[string]string{
					"input.txt": "12345",
				},
			},
		},
		{
			Name: "Compile with TTY",
			Input: Cmd{
				Args: []string{"/usr/bin/g++", "a.cc"},
				Env:  []string{"PATH=/usr/bin:/bin", "TERM=xterm"},
				Tty:  true,
				Files: []*CmdFile{
					{Content: "/dev/null"},       // stdin
					{Name: "stdout", Max: 10240}, // stdout
					{Name: "stderr", Max: 10240}, // stderr
				},
				CopyIn: map[string]CmdFile{
					"a.cc": {Content: "int main(){int a}"},
				},
			},
			Expect: Expectation{
				Status: "Nonzero Exit Status",
				FilesContains: map[string]string{
					"stdout": "\033",
				},
			},
		},
	}

	// 5. Execution Loop
	client := &http.Client{Timeout: 10 * time.Second}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			// A. Merge Defaults
			cmd := mergeDefaults(baseCmd, tc.Input)

			// B. Send Request
			reqBody := Request{Cmd: []Cmd{cmd}}
			jsonBytes, _ := json.Marshal(reqBody)

			resp, err := client.Post(serverURL, "application/json", bytes.NewBuffer(jsonBytes))
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("Server returned %d: %s", resp.StatusCode, string(body))
			}

			// C. Verify
			var results []Result
			if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
				t.Fatalf("Decode failed: %v", err)
			}
			res := results[0]

			// Check Status
			if tc.Expect.Status != "" && res.Status != tc.Expect.Status {
				t.Errorf("Status mismatch.\nExpected: %s\nActual:   %s\nError:    %s",
					tc.Expect.Status, res.Status, res.Error)
			}

			// Check Error Message
			if tc.Expect.ErrorContains != "" && !strings.Contains(res.Error, tc.Expect.ErrorContains) {
				t.Errorf("Error message mismatch.\nExpected substring: %s\nActual:             %s",
					tc.Expect.ErrorContains, res.Error)
			}

			// Check File Content
			for file, expectedContent := range tc.Expect.FilesContains {
				actualContent := strings.TrimSpace(res.Files[file])
				if !strings.Contains(actualContent, expectedContent) {
					t.Errorf("File [%s] mismatch.\nExpected content: %s\nActual content:   %s",
						file, expectedContent, actualContent)
				}
			}

			for file, expectedID := range tc.Expect.FileIDs {
				actualID, ok := res.FileIDs[file]
				if !ok {
					t.Errorf("FileID [%s] missing", file)
					continue
				}
				if expectedID != "" && actualID != expectedID {
					t.Errorf("FileID [%s] mismatch.\nExpected: %s\nActual:   %s",
						file, expectedID, actualID)
				}
				if expectedID == "" && actualID == "" {
					t.Errorf("FileID [%s] should not be empty", file)
				}
			}

			for file, fileID := range res.FileIDs {
				if fileID == "" {
					continue
				}
				deleteReq, err := http.NewRequest(http.MethodDelete, fileURL+fileID, nil)
				if err != nil {
					t.Fatalf("failed to construct delete request for %s: %v", file, err)
				}
				deleteResp, err := client.Do(deleteReq)
				if err != nil {
					t.Fatalf("failed to delete cached file %s: %v", fileID, err)
				}
				if deleteResp.Body != nil {
					deleteResp.Body.Close()
				}
				if deleteResp.StatusCode != http.StatusOK {
					t.Fatalf("delete cached file %s returned %d", fileID, deleteResp.StatusCode)
				}
			}
		})
	}
}

// Helper to overlay the test case config on top of the base defaults
func mergeDefaults(base, override Cmd) Cmd {
	res := base

	// Merge simple fields if they are provided in override
	if len(override.Args) > 0 {
		res.Args = override.Args
	}
	if len(override.Env) > 0 {
		res.Env = override.Env
	}
	if override.Tty {
		res.Tty = override.Tty
	}
	if override.CopyIn != nil {
		res.CopyIn = override.CopyIn
	}
	if override.CopyOut != nil {
		res.CopyOut = override.CopyOut
	}
	if override.CopyOutCached != nil {
		res.CopyOutCached = override.CopyOutCached
	}

	// Merge Limits (only if non-zero)
	if override.CPULimit > 0 {
		res.CPULimit = override.CPULimit
	}
	if override.RealCPULimit > 0 {
		res.RealCPULimit = override.RealCPULimit
	}
	if override.MemoryLimit > 0 {
		res.MemoryLimit = override.MemoryLimit
	}
	if override.StackLimit > 0 {
		res.StackLimit = override.StackLimit
	}

	if override.CopyOutMax > 0 {
		res.CopyOutMax = override.CopyOutMax
	}
	if override.CopyOutTruncate {
		res.CopyOutTruncate = override.CopyOutTruncate
	}
	if override.Files != nil {
		res.Files = override.Files
	}

	return res
}
