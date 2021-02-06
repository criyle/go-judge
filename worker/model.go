package worker

import (
	"fmt"
	"time"

	"github.com/criyle/go-judge/envexec"
)

// Cmd defines command and limits to start a program using in envexec
type Cmd struct {
	Args  []string
	Env   []string
	Files []CmdFile
	TTY   bool

	CPULimit          time.Duration
	ClockLimit        time.Duration
	MemoryLimit       envexec.Size
	StackLimit        envexec.Size
	ProcLimit         uint64
	CPURateLimit      float64
	StrictMemoryLimit bool

	CopyIn map[string]CmdFile

	CopyOut       []string
	CopyOutCached []string
	CopyOutMax    uint64
	CopyOutDir    string
}

// PipeIndex defines indexing for a pipe fd
type PipeIndex struct {
	Index int
	Fd    int
}

// PipeMap defines in / out pipe for multiple program
type PipeMap struct {
	In  PipeIndex
	Out PipeIndex
}

// Request defines single worker request
type Request struct {
	RequestID   string
	Cmd         []Cmd
	PipeMapping []PipeMap
}

// Result defines single command response
type Result struct {
	Status     envexec.Status
	ExitStatus int
	Error      string
	Time       time.Duration
	RunTime    time.Duration
	Memory     envexec.Size
	Files      map[string][]byte
	FileIDs    map[string]string
}

// Response defines worker response for single request
type Response struct {
	RequestID string
	Results   []Result
	Error     error
}

func (r Result) String() string {
	type Result struct {
		Status     envexec.Status
		ExitStatus int
		Error      string
		Time       time.Duration
		RunTime    time.Duration
		Memory     envexec.Size
		Files      map[string]string
		FileIDs    map[string]string
	}
	d := Result{
		Status:     r.Status,
		ExitStatus: r.ExitStatus,
		Error:      r.Error,
		Time:       r.Time,
		RunTime:    r.RunTime,
		Memory:     r.Memory,
		Files:      make(map[string]string),
		FileIDs:    r.FileIDs,
	}
	for k, v := range r.Files {
		d.Files[k] = fmt.Sprintf("(len:%d)", len(v))
	}
	return fmt.Sprintf("%+v", d)
}
