package worker

import "github.com/criyle/go-judge/pkg/envexec"

// CmdFile defines file from multiple source including local / memory / cached or pipe collector
type CmdFile struct {
	Src     *string `json:"src"`
	Content *string `json:"content"`
	FileID  *string `json:"fileId"`
	Name    *string `json:"name"`
	Max     *int64  `json:"max"`
}

// Cmd defines command and limits to start a program using in envexec
type Cmd struct {
	Args  []string   `json:"args"`
	Env   []string   `json:"env,omitempty"`
	Files []*CmdFile `json:"files,omitempty"`

	CPULimit     uint64 `json:"cpuLimit"`
	RealCPULimit uint64 `json:"realCpuLimit"`
	MemoryLimit  uint64 `json:"memoryLimit"`
	ProcLimit    uint64 `json:"procLimit"`

	CopyIn map[string]CmdFile `json:"copyIn"`

	CopyOut       []string `json:"copyOut"`
	CopyOutCached []string `json:"copyOutCached"`
	CopyOutDir    string   `json:"copyOutDir"`
}

// PipeIndex defines indexing for a pipe fd
type PipeIndex struct {
	Index int `json:"index"`
	Fd    int `json:"fd"`
}

// PipeMap defines in / out pipe for multiple program
type PipeMap struct {
	In  PipeIndex `json:"in"`
	Out PipeIndex `json:"out"`
}

// Request defines single worker request
type Request struct {
	RequestID   string    `json:"requestId"`
	Cmd         []Cmd     `json:"cmd"`
	PipeMapping []PipeMap `json:"pipeMapping"`
}

// Status offers JSON marshal for envexec.Status
type Status envexec.Status

// MarshalJSON convert status into string
func (s Status) MarshalJSON() ([]byte, error) {
	return []byte("\"" + (envexec.Status)(s).String() + "\""), nil
}

// Response defines single command response
type Response struct {
	Status     Status            `json:"status"`
	ExitStatus int               `json:"exitStatus"`
	Error      string            `json:"error,omitempty"`
	Time       uint64            `json:"time"`
	Memory     uint64            `json:"memory"`
	Files      map[string]string `json:"files,omitempty"`
	FileIDs    map[string]string `json:"fileIds,omitempty"`
}

// Result defines worker response for single request
type Result struct {
	RequestID string     `json:"requestId"`
	Response  []Response `json:"results"`
	Error     error      `json:"-"`
	ErrorMsg  string     `json:"error,omitempty"`
}
