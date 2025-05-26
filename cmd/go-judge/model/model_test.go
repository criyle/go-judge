package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/criyle/go-judge/worker"
)

func TestStatus_MarshalUnmarshalJSON(t *testing.T) {
	type wrap struct {
		Status Status `json:"status"`
	}
	orig := wrap{Status: 1}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var got wrap
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if got.Status != orig.Status {
		t.Errorf("got %v, want %v", got.Status, orig.Status)
	}
}

func TestStatus_UnmarshalJSON_Invalid(t *testing.T) {
	var s Status
	err := s.UnmarshalJSON([]byte(`"not_a_status"`))
	if err == nil {
		t.Error("expected error for invalid status string")
	}
}

func TestConvertCopyOut(t *testing.T) {
	in := []string{"foo.txt", "bar.txt?"}
	out := convertCopyOut(in)
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
	if out[0].Name != "foo.txt" || out[0].Optional {
		t.Errorf("unexpected: %+v", out[0])
	}
	if out[1].Name != "bar.txt" || !out[1].Optional {
		t.Errorf("unexpected: %+v", out[1])
	}
}

func TestCheckPathPrefixes(t *testing.T) {
	tmp := t.TempDir()
	abs := filepath.Join(tmp, "file.txt")
	os.WriteFile(abs, []byte("x"), 0644)
	ok, err := CheckPathPrefixes(abs, []string{tmp})
	if err != nil {
		t.Fatalf("CheckPathPrefixes error: %v", err)
	}
	if !ok {
		t.Errorf("expected true for prefix match")
	}
	ok, err = CheckPathPrefixes(abs, []string{"/not/a/prefix"})
	if err != nil {
		t.Fatalf("CheckPathPrefixes error: %v", err)
	}
	if ok {
		t.Errorf("expected false for non-matching prefix")
	}
}

func TestConvertCmdFile_Local(t *testing.T) {
	src := "/tmp/foo"
	f := &CmdFile{Src: &src}
	_, err := convertCmdFile(f, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConvertCmdFile_Content(t *testing.T) {
	content := "abc"
	f := &CmdFile{Content: &content}
	cf, err := convertCmdFile(f, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if cf == nil {
		t.Error("expected non-nil CmdFile")
	}
}

func TestConvertCmdFile_FileID(t *testing.T) {
	id := "id"
	f := &CmdFile{FileID: &id}
	cf, err := convertCmdFile(f, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if cf == nil {
		t.Error("expected non-nil CmdFile")
	}
}

func TestConvertCmdFile_Collector(t *testing.T) {
	name := "out"
	max := int64(123)
	f := &CmdFile{Name: &name, Max: &max}
	cf, err := convertCmdFile(f, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if cf == nil {
		t.Error("expected non-nil CmdFile")
	}
}

func TestConvertCmdFile_Invalid(t *testing.T) {
	f := &CmdFile{}
	_, err := convertCmdFile(f, nil)
	if err == nil {
		t.Error("expected error for invalid CmdFile")
	}
}

func TestResult_String(t *testing.T) {
	r := Result{
		Status:     1,
		ExitStatus: 0,
		Error:      "",
		Time:       uint64(time.Second),
		RunTime:    uint64(time.Second),
		Memory:     1024,
		Files:      map[string]string{"foo": "bar"},
	}
	s := r.String()
	if s == "" {
		t.Error("expected non-empty string")
	}
}

func TestConvertPipe(t *testing.T) {
	p := PipeMap{
		In:    PipeIndex{Index: 1, Fd: 2},
		Out:   PipeIndex{Index: 3, Fd: 4},
		Name:  "pipe",
		Max:   100,
		Proxy: true,
	}
	wp := convertPipe(p)
	if wp.In.Index != 1 || wp.Out.Fd != 4 || wp.Name != "pipe" || wp.Limit != 100 {
		t.Errorf("unexpected convertPipe result: %+v", wp)
	}
}

func TestConvertRequest_Basic(t *testing.T) {
	src := "/tmp/foo"
	content := "abc"
	fileID := "id"
	name := "out"
	max := int64(123)
	copyOut := []string{"result.txt", "log.txt?"}
	req := &Request{
		Cmd: []Cmd{{
			Args:        []string{"echo", "hello"},
			Files:       []*CmdFile{{Src: &src}, {Content: &content}, {FileID: &fileID}, {Name: &name, Max: &max}},
			CopyOut:     copyOut,
			CPULimit:    uint64(1000 * time.Millisecond),
			MemoryLimit: 1024,
		}},
	}
	workerReq, err := ConvertRequest(req, []string{"/tmp"})
	if err != nil {
		t.Fatalf("ConvertRequest error: %v", err)
	}
	if len(workerReq.Cmd[0].Files) != 4 {
		t.Errorf("expected 4 files, got %d", len(workerReq.Cmd[0].Files))
	}
	if len(workerReq.Cmd[0].CopyOut) != 2 {
		t.Errorf("expected 2 copyOut, got %d", len(workerReq.Cmd[0].CopyOut))
	}
	if workerReq.Cmd[0].CPULimit != 1000*time.Millisecond {
		t.Errorf("unexpected CPULimit: %v", workerReq.Cmd[0].CPULimit)
	}
	if workerReq.Cmd[0].MemoryLimit != 1024 {
		t.Errorf("unexpected MemoryLimit: %v", workerReq.Cmd[0].MemoryLimit)
	}
}

func TestConvertRequest_InvalidFile(t *testing.T) {
	req := &Request{
		Cmd: []Cmd{
			{
				Files: []*CmdFile{{}}, // invalid
			},
		},
	}
	_, err := ConvertRequest(req, nil)
	if err == nil {
		t.Error("expected error for invalid CmdFile")
	}
}

func TestConvertResponse_Basic(t *testing.T) {
	res := worker.Response{
		Results: []worker.Result{{
			Status:     1,
			ExitStatus: 0,
			Error:      "",
			Time:       1000 * time.Millisecond,
			RunTime:    900 * time.Millisecond,
			Memory:     2048,
			ProcPeak:   2,
			Files:      map[string]*os.File{},
			FileError:  []worker.FileError{{Name: "foo", Type: 1, Message: "err"}},
		}},
	}
	resp, _ := ConvertResponse(res, false)
	if resp.Results[0].Status != 1 {
		t.Errorf("unexpected Status: %v", resp.Results[0].Status)
	}
	if resp.Results[0].Time != uint64(1000*time.Millisecond) {
		t.Errorf("unexpected Time: %v", resp.Results[0].Time)
	}
	if len(resp.Results[0].FileError) != 1 {
		t.Errorf("unexpected FileError: %+v", resp.Results[0].FileError)
	}
}
