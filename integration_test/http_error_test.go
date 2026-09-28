//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"
)

func TestRunCopyInHTTPFileErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.txt")
	tests := []struct {
		name       string
		copyIn     map[string]CmdFile
		wantStatus int
	}{
		{
			name:       "missing local source",
			copyIn:     map[string]CmdFile{"input.txt": {Src: missing}},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "malformed source file",
			copyIn:     map[string]CmdFile{"input.txt": {}},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := Request{Cmd: []Cmd{{
				Args:     []string{"/bin/true"},
				Files:    []*CmdFile{{Src: "/dev/null"}, {Name: "stdout", Max: 1024}, {Name: "stderr", Max: 1024}},
				CopyIn:   tc.copyIn,
				CPULimit: 1_000_000_000,
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
			if resp.StatusCode != tc.wantStatus {
				responseBody, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected HTTP %d, got %d: %s", tc.wantStatus, resp.StatusCode, responseBody)
			}
		})
	}
}
