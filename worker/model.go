package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/criyle/go-judge/envexec"
)

type Size = envexec.Size
type CmdCopyOutFile = envexec.CmdCopyOutFile
type PipeMap = envexec.Pipe
type PipeIndex = envexec.PipeIndex

// Cmd defines command and limits to start a program using in envexec
type Cmd struct {
	Args  []string
	Env   []string
	Files []CmdFile
	TTY   bool

	CPULimit          time.Duration
	ClockLimit        time.Duration
	MemoryLimit       Size
	StackLimit        Size
	OutputLimit       Size
	ProcLimit         uint64
	OpenFileLimit     uint64
	CPURateLimit      uint64
	CPUSetLimit       string
	StrictMemoryLimit bool

	CopyIn map[string]CmdFile

	CopyOut       []CmdCopyOutFile
	CopyOutCached []CmdCopyOutFile
	CopyOutMax    uint64
	CopyOutDir    string
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
	Files      map[string]*os.File
	FileIDs    map[string]string
	FileError  []envexec.FileError
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
		FileError  []envexec.FileError
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
		FileError:  r.FileError,
	}
	for k, v := range r.Files {
		d.Files[k] = filepath.Base(v.Name())
	}
	return fmt.Sprintf("%+v", d)
}
