//go:build integration

package integration_test

const serverURL = "http://localhost:5050/run"
const fileURL = "http://localhost:5050/file/"
const dataSize = 1024 * 1024 // 1MB payload

type CmdFile struct {
	Src     string `json:"src,omitempty"`
	Content string `json:"content,omitempty"`
	FileID  string `json:"fileId,omitempty"`
	Name    string `json:"name,omitempty"`
	Max     int64  `json:"max,omitempty"`
	Symlink string `json:"symlink,omitempty"`
}

type Cmd struct {
	Args  []string   `json:"args"`
	Env   []string   `json:"env,omitempty"`
	Files []*CmdFile `json:"files,omitempty"`

	Tty bool `json:"tty,omitempty"`

	CPULimit        uint64 `json:"cpuLimit"`
	RealCPULimit    uint64 `json:"realCpuLimit"`
	MemoryLimit     uint64 `json:"memoryLimit"`
	StackLimit      uint64 `json:"stackLimit"`
	ProcLimit       uint64 `json:"procLimit"`
	CPUSetLimit     string `json:"cpuSetLimit,omitempty"`
	CopyOutMax      uint64 `json:"copyOutMax,omitempty"`
	CopyOutTruncate bool   `json:"copyOutTruncate,omitempty"`

	CopyIn        map[string]CmdFile `json:"copyIn,omitempty"`
	CopyOut       []string           `json:"copyOut,omitempty"`
	CopyOutCached []string           `json:"copyOutCached,omitempty"`
}

type Request struct {
	Cmd         []Cmd     `json:"cmd"`
	PipeMapping []PipeMap `json:"pipeMapping,omitempty"`
}

type PipeMap struct {
	In              PipeIndex `json:"in"`
	Out             PipeIndex `json:"out"`
	Proxy           bool      `json:"proxy,omitempty"`
	DisableZeroCopy bool      `json:"disableZeroCopy,omitempty"`
}

type PipeIndex struct {
	Index int `json:"index"`
	Fd    int `json:"fd"`
}

type Result struct {
	Status  string            `json:"status"`
	Time    uint64            `json:"time"`
	RunTime uint64            `json:"runTime"`
	Files   map[string]string `json:"files"`
	FileIDs map[string]string `json:"fileIds"`
	Error   string            `json:"error"`
}
