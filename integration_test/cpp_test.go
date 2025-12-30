//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCompileAndRun_CPP(t *testing.T) {
	const serverURL = "http://localhost:5050/run"
	client := &http.Client{Timeout: 15 * time.Second}

	// --- Step 1: Compile C++ Source ---
	t.Log("Step 1: Compiling C++ code...")

	sourceCode := `#include <iostream>
#include <cstdio>
#include <unistd.h>
using namespace std;
int main() {
    int a, b;
    if (cin >> a >> b) {
        cout << a + b;
    }
    return 0;
}`

	compileReq := Request{
		Cmd: []Cmd{{
			Args: []string{"/usr/bin/g++", "-O2", "a.cc", "-o", "a", "-std=c++11", "-lm"},
			Env:  []string{"PATH=/usr/bin:/bin"},
			Files: []*CmdFile{
				{Content: "test"},
				{Name: "stdout", Max: 10240},
				{Name: "stderr", Max: 10240},
			},
			CPULimit:    3 * 1000 * 1000 * 1000,
			MemoryLimit: 256 * 1024 * 1024,
			ProcLimit:   50,
			CopyIn: map[string]CmdFile{
				"a.cc": {Content: sourceCode},
			},
			CopyOutCached: []string{"a"}, // Crucial: Cache the binary named "a"
		}},
	}

	compileBody, _ := json.Marshal(compileReq)
	resp, err := client.Post(serverURL, "application/json", bytes.NewBuffer(compileBody))
	if err != nil {
		t.Fatalf("Compile request failed: %v", err)
	}
	defer resp.Body.Close()

	var compileResults []Result
	if err := json.NewDecoder(resp.Body).Decode(&compileResults); err != nil {
		t.Fatalf("Failed to decode compile response: %v", err)
	}

	// Verify Compilation Success
	cRes := compileResults[0]
	if cRes.Status != "Accepted" {
		t.Fatalf("Compilation Failed: %s\nStderr: %s", cRes.Status, cRes.Files["stderr"])
	}

	// Extract File ID
	executableID, ok := cRes.FileIDs["a"]
	if !ok || executableID == "" {
		t.Fatalf("Server did not return a File ID for the cached binary 'a'. Response: %+v", cRes.FileIDs)
	}
	t.Logf("Compilation successful. Cached Executable ID: %s", executableID)

	// --- Step 2: Run Cached Binary ---
	t.Log("Step 2: Running cached binary...")

	runReq := Request{
		Cmd: []Cmd{{
			Args: []string{"./a"}, // Execute the file we copy in
			Env:  []string{"PATH=/usr/bin:/bin"},
			Files: []*CmdFile{
				{Content: "1 1"},             // Stdin input
				{Name: "stdout", Max: 10240}, // Capture stdout
				{Name: "stderr", Max: 10240},
			},
			CPULimit:    1 * 1000 * 1000 * 1000,
			MemoryLimit: 128 * 1024 * 1024,
			ProcLimit:   50,
			CopyIn: map[string]CmdFile{
				"a": {FileID: executableID}, // Use the ID from Step 1
			},
		}},
	}

	runBody, _ := json.Marshal(runReq)
	respRun, err := client.Post(serverURL, "application/json", bytes.NewBuffer(runBody))
	if err != nil {
		t.Fatalf("Run request failed: %v", err)
	}
	defer respRun.Body.Close()

	var runResults []Result
	if err := json.NewDecoder(respRun.Body).Decode(&runResults); err != nil {
		t.Fatalf("Failed to decode run response: %v", err)
	}

	// Verify Run Success
	rRes := runResults[0]
	if rRes.Status != "Accepted" {
		t.Fatalf("Run Execution Failed: %s\nStderr: %s", rRes.Status, rRes.Files["stderr"])
	}

	// Verify Output
	expectedOutput := "2"
	actualOutput := strings.TrimSpace(rRes.Files["stdout"])

	if actualOutput != expectedOutput {
		t.Errorf("Wrong Output.\nExpected: %s\nActual:   %s", expectedOutput, actualOutput)
	} else {
		t.Log("Run successful. Output matched.")
	}

	t.Logf("Delete File: %s", executableID)
	deleteReq, err := http.NewRequest("DELETE", "http://localhost:5050/file/"+executableID, nil)
	if err != nil {
		t.Fatalf("Failed to construct delete request")
	}
	resp, err = client.Do(deleteReq)
	if err != nil {
		t.Fatalf("Failed to send delete request")
	}
}
