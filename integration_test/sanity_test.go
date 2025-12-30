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

type SanityCmd struct {
	Args  []string  `json:"args"`
	Env   []string  `json:"env,omitempty"`
	Files []CmdFile `json:"files,omitempty"`

	Tty bool `json:"tty,omitempty"`

	CPULimit     uint64 `json:"cpuLimit"`
	RealCPULimit uint64 `json:"realCpuLimit"`
	MemoryLimit  uint64 `json:"memoryLimit"`
	StackLimit   uint64 `json:"stackLimit"`
	ProcLimit    uint64 `json:"procLimit"`

	CopyIn  map[string]SanityFile `json:"copyIn,omitempty"`
	CopyOut []string              `json:"copyOut,omitempty"`

	CopyOutMax      uint64 `json:"copyOutMax,omitempty"`
	CopyOutTruncate bool   `json:"copyOutTruncate,omitempty"`
}

type SanityFile struct {
	Content string `json:"content,omitempty"`
}

type SanityRequest struct {
	Cmd []SanityCmd `json:"cmd"`
}

type SanityResult struct {
	Status string            `json:"status"`
	Error  string            `json:"error"`
	Files  map[string]string `json:"files"`
}

func TestSanity_BasicFunctionality(t *testing.T) {
	const serverURL = "http://localhost:5050/run"

	// 1. Define the Expected Behavior
	type Expectation struct {
		Status        string
		ErrorContains string            // Substring check for the error message
		FilesContains map[string]string // Check if stdout/stderr contains specific text
	}

	// 2. Define the Test Case Structure
	type TestCase struct {
		Name   string
		Input  SanityCmd
		Expect Expectation
	}

	// 3. Define the Default Configuration (Base Template)
	baseCmd := SanityCmd{
		Env: []string{"PATH=/usr/bin:/bin"},
		Files: []CmdFile{
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
			Input: SanityCmd{
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
			Input: SanityCmd{
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
			Input: SanityCmd{
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
			Input: SanityCmd{
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
			Input: SanityCmd{
				Args: []string{"/bin/bash", "-c", "kill -SIGINT 1"},
			},
			Expect: Expectation{
				Status: "Accepted",
			},
		},
		{
			Name: "Copy in Sub Directory",
			Input: SanityCmd{
				Args: []string{"/bin/ls", "test_dir"},
				CopyIn: map[string]SanityFile{
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
			Input: SanityCmd{
				Args: []string{"/bin/ls", "/tmp"},
				CopyIn: map[string]SanityFile{
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
			Input: SanityCmd{
				Args:    []string{"/bin/ls"},
				CopyOut: []string{"test"},
			},
			Expect: Expectation{
				Status: "File Error",
			},
		},
		{
			Name: "Copy out Optional File Accepted",
			Input: SanityCmd{
				Args:    []string{"/bin/ls"},
				CopyOut: []string{"test?"},
			},
			Expect: Expectation{
				Status: "Accepted",
			},
		},
		{
			Name: "Stack Limit",
			Input: SanityCmd{
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
			Input: SanityCmd{
				Args:    []string{"/bin/ln", "-s", "/etc/passwd", "out.txt"},
				CopyOut: []string{"out.txt"},
			},
			Expect: Expectation{
				Status: "File Error",
			},
		},
		{
			Name: "Copy out max",
			Input: SanityCmd{
				Args: []string{"/bin/cat", "input.txt"},
				CopyIn: map[string]SanityFile{
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
			Input: SanityCmd{
				Args: []string{"/bin/cat", "input.txt"},
				CopyIn: map[string]SanityFile{
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
			Input: SanityCmd{
				Args: []string{"/bin/g++", "a.cc"},
				Env:  []string{"PATH=/usr/bin:/bin", "TERM=xterm"},
				Tty:  true,
				Files: []CmdFile{
					{Content: "/dev/null"},       // stdin
					{Name: "stdout", Max: 10240}, // stdout
					{Name: "stderr", Max: 10240}, // stderr
				},
				CopyIn: map[string]SanityFile{
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
			reqBody := SanityRequest{Cmd: []SanityCmd{cmd}}
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
			var results []SanityResult
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
		})
	}
}

// Helper to overlay the test case config on top of the base defaults
func mergeDefaults(base, override SanityCmd) SanityCmd {
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
