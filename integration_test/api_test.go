//go:build integration

package integration_test

// const serverURL = "http://192.168.3.30:5050/run"
const serverURL = "http://localhost:5050/run"
const dataSize = 1024 * 1024 // 1MB payload

// Extended Cmd struct to support pipe mapping logic if your API supports it directly.
// Note: Based on standard go-judge schemas, interaction is often handled by
// passing multiple Cms in one request and defining file mappings.
type Cmd struct {
	Args  []string   `json:"args"`
	Env   []string   `json:"env,omitempty"`
	Files []*CmdFile `json:"files,omitempty"` // file descriptors

	CPULimit    uint64 `json:"cpuLimit"`
	MemoryLimit uint64 `json:"memoryLimit"`
	ProcLimit   uint64 `json:"procLimit"`

	CopyIn        map[string]CmdFile `json:"copyIn"`
	CopyOutCached []string           `json:"copyOutCached"`
}

type CmdFile struct {
	Src     string `json:"src,omitempty"`
	Content string `json:"content,omitempty"`
	FileID  string `json:"fileId,omitempty"`
	Name    string `json:"name,omitempty"`
	Max     int64  `json:"max,omitempty"`
}

// Request structure supporting pipe mapping
type Request struct {
	Cmd []Cmd `json:"cmd"`
	// PipeMapping defines how FDs are connected between commands.
	// Format: {In: {Index: 0, Fd: 1}, Out: {Index: 1, Fd: 0}}
	// This connects Cmd[0].stdout (FD 1) -> Cmd[1].stdin (FD 0)
	PipeMapping []PipeMap `json:"pipeMapping"`
}

type PipeMap struct {
	In  PipeIndex `json:"in"`
	Out PipeIndex `json:"out"`
	// Optional: if your system supports a proxy/buffer limit
	Proxy bool `json:"proxy,omitempty"`
}

type PipeIndex struct {
	Index int `json:"index"` // Index of the command in the Cmd array
	Fd    int `json:"fd"`    // File Descriptor number (0=stdin, 1=stdout)
}

type Result struct {
	Status  string            `json:"status"`
	Files   map[string]string `json:"files"`
	FileIDs map[string]string `json:"fileIds"`
	Error   string            `json:"error"`
}
