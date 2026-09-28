package restexecutor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/criyle/go-judge/cmd/go-judge/model"
	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/worker"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap/zaptest"
)

// mockWorker is a mock implementation of the worker.Worker interface
type mockWorker struct {
	// The result to send back when Submit is called
	Result worker.Result
	Error  error
	worker.Worker
}

func (m *mockWorker) Submit(_ context.Context, req *worker.Request) (<-chan worker.Response, <-chan struct{}) {
	// Mock implementation
	rtCh := make(chan worker.Response, 1)
	rtCh <- worker.Response{
		RequestID: req.RequestID,
		Results:   []worker.Result{m.Result},
		Error:     m.Error,
	}
	return rtCh, nil
}

func TestHandleRunMissingFileReturnsNotFound(t *testing.T) {
	router := gin.New()
	cmdHandle := NewCmdHandle(&mockWorker{}, nil, zaptest.NewLogger(t))
	cmdHandle.Register(router)

	src := filepath.Join(t.TempDir(), "missing.txt")
	body, err := json.Marshal(model.Request{Cmd: []model.Cmd{{
		Args:   []string{"/bin/true"},
		CopyIn: map[string]model.CmdFile{"input.txt": {Src: &src}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, recorder.Code)
	}
}

func TestHandleRunOtherWorkerErrorReturnsInternalServerError(t *testing.T) {
	router := gin.New()
	cmdHandle := NewCmdHandle(&mockWorker{Error: fmt.Errorf("container unavailable")}, nil, zaptest.NewLogger(t))
	cmdHandle.Register(router)

	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader([]byte(`{"cmd":[{"args":["/bin/true"]}]}`)))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, recorder.Code)
	}
}

// requestToReader converts a model.Request to an io.Reader
func requestToReader(req model.Request) io.Reader {
	// Convert the request to JSON
	data, err := json.Marshal(req)
	if err != nil {
		return nil
	}

	// Create a new reader from the JSON data
	return io.NopCloser(bytes.NewReader(data))
}

// assertWorkerResultEqualsModelResult checks if two worker.Result are equal
func assertWorkerResultEqualsModelResult(a worker.Result, b model.Result) error {
	if a.Status.String() != b.Status.String() {
		return fmt.Errorf("expected status %s, got %s", a.Status.String(), b.Status.String())
	}
	if a.ExitStatus != b.ExitStatus {
		return fmt.Errorf("expected exit status %d, got %d", a.ExitStatus, b.ExitStatus)
	}
	if a.Error != b.Error {
		return fmt.Errorf("expected error %s, got %s", a.Error, b.Error)
	}
	if a.Time != time.Duration(b.Time) {
		return fmt.Errorf("expected time %s, got %s", a.Time, time.Duration(b.Time))
	}
	if a.Memory != worker.Size(b.Memory) {
		return fmt.Errorf("expected memory %d, got %d", a.Memory, worker.Size(b.Memory))
	}
	if a.RunTime != time.Duration(b.RunTime) {
		return fmt.Errorf("expected run time %s, got %s", a.RunTime, time.Duration(b.RunTime))
	}
	if a.ProcPeak != b.ProcPeak {
		return fmt.Errorf("expected proc peak %d, got %d", a.ProcPeak, b.ProcPeak)
	}
	if !maps.Equal(a.FileIDs, b.FileIDs) {
		return fmt.Errorf("expected file IDs %v, got %v", a.FileIDs, b.FileIDs)
	}
	if !slices.Equal(a.FileError, b.FileError) {
		return fmt.Errorf("expected file errors %v, got %v", a.FileError, b.FileError)
	}
	return nil
}

// TestHandleRun tests the handleRun method of the cmdHandle
func TestHandleRun(t *testing.T) {
	// Create a new Gin router
	router := gin.Default()
	// Create a mock worker
	mockWorker := &mockWorker{
		Result: worker.Result{
			Status:     envexec.StatusAccepted,
			ExitStatus: 0,
			Error:      "",
			Time:       time.Millisecond * 30,
			Memory:     32243712,
			RunTime:    time.Millisecond * 52,
			FileIDs: map[string]string{
				"a":    "5LWIZAA45JHX4Y4Z",
				"a.cc": "NOHPGGDTYQUFRSLJ",
			},
		},
	}
	// Create a logger
	logger := zaptest.NewLogger(t)
	// Create a new command handle
	cmdHandle := NewCmdHandle(mockWorker, nil, logger)
	cmdHandle.Register(router)

	// Create a test request
	req := model.Request{
		RequestID: "qwq",
		Cmd: []model.Cmd{
			{
				Args: []string{"/usr/bin/g++", "a.cc", "-o", "a"},
				Env:  []string{"PATH=/usr/bin:/bin"},
				Files: []*model.CmdFile{
					{
						Content: new(""),
					}, {
						Name: new("stdout"),
						Max:  new(int64(10240)),
					}, {
						Name: new("stderr"),
						Max:  new(int64(10240)),
					}},
				CPULimit:    10000000000,
				MemoryLimit: 104857600,
				ProcLimit:   50,
				CopyIn: map[string]model.CmdFile{
					"a.cc": {
						Content: new("#include <iostream>\nusing namespace std;\nint main() {\nint a, b;\ncin >> a >> b;\ncout << a + b << endl;\n}"),
					},
				},
				CopyOut:       []string{"stdout", "stderr"},
				CopyOutCached: []string{"a.cc", "a"},
			},
		},
	}

	// Convert the request to a reader
	requestBody := requestToReader(req)

	// Create a test HTTP request
	testReq := httptest.NewRequest("POST", "/run", requestBody)
	// Set the request context
	testReq = testReq.WithContext(context.Background())
	// Set the request header
	testReq.Header.Set("Content-Type", "application/json")

	// Create a test HTTP recorder
	recorder := httptest.NewRecorder()
	// Serve the HTTP request
	router.ServeHTTP(recorder, testReq)
	// Check the response status code
	if recorder.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", 200, recorder.Code)
	}
	// Check the response body
	responseBody := recorder.Body.String()
	if responseBody == "" {
		t.Fatalf("Expected non-empty response body, got empty")
	}
	// Print the response body
	t.Logf("Response body: %s", responseBody)
	// Check if the response is valid JSON
	var response []model.Result
	if err := json.Unmarshal([]byte(responseBody), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(response) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(response))
	}
	// Check if the response matches the expected result
	if err := assertWorkerResultEqualsModelResult(mockWorker.Result, response[0]); err != nil {
		t.Fatalf("Expected result to match, but got error: %v", err)
	}
}
