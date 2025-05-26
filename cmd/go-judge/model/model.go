package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/worker"
)

// FileError defines the location, file name and the detailed message for a failed file operation
type FileError = envexec.FileError

// FileErrorType defines the location that file operation fails
type FileErrorType = envexec.FileErrorType

// CmdFile defines file from multiple source including local / memory / cached or pipe collector
type CmdFile struct {
	Src       *string `json:"src"`
	Content   *string `json:"content"`
	FileID    *string `json:"fileId"`
	Name      *string `json:"name"`
	Max       *int64  `json:"max"`
	Symlink   *string `json:"symlink"`
	StreamIn  bool    `json:"streamIn"`
	StreamOut bool    `json:"streamOut"`
	Pipe      bool    `json:"pipe"`
}

// Cmd defines command and limits to start a program using in envexec
type Cmd struct {
	Args  []string   `json:"args"`
	Env   []string   `json:"env,omitempty"`
	Files []*CmdFile `json:"files,omitempty"`

	CPULimit     uint64 `json:"cpuLimit"`
	RealCPULimit uint64 `json:"realCpuLimit"`
	ClockLimit   uint64 `json:"clockLimit"`
	MemoryLimit  uint64 `json:"memoryLimit"`
	StackLimit   uint64 `json:"stackLimit"`
	ProcLimit    uint64 `json:"procLimit"`
	CPURateLimit uint64 `json:"cpuRateLimit"`
	CPUSetLimit  string `json:"cpuSetLimit"`

	CopyIn map[string]CmdFile `json:"copyIn"`

	CopyOut       []string `json:"copyOut"`
	CopyOutCached []string `json:"copyOutCached"`
	CopyOutMax    uint64   `json:"copyOutMax"`
	CopyOutDir    string   `json:"copyOutDir"`

	TTY               bool `json:"tty,omitempty"`
	StrictMemoryLimit bool `json:"strictMemoryLimit"`
	DataSegmentLimit  bool `json:"dataSegmentLimit"`
	AddressSpaceLimit bool `json:"addressSpaceLimit"`
}

// PipeIndex defines indexing for a pipe fd
type PipeIndex struct {
	Index int `json:"index"`
	Fd    int `json:"fd"`
}

// PipeMap defines in / out pipe for multiple program
type PipeMap struct {
	In    PipeIndex `json:"in"`
	Out   PipeIndex `json:"out"`
	Name  string    `json:"name"`
	Max   int64     `json:"max"`
	Proxy bool      `json:"proxy"`
}

// Request defines single worker request
type Request struct {
	RequestID   string    `json:"requestId"`
	Cmd         []Cmd     `json:"cmd"`
	PipeMapping []PipeMap `json:"pipeMapping"`
}

// Status offers JSON marshal for envexec.Status
type Status envexec.Status

// String converts status to string
func (s Status) String() string {
	return envexec.Status(s).String()
}

// MarshalJSON convert status into string
func (s Status) MarshalJSON() ([]byte, error) {
	return []byte("\"" + envexec.Status(s).String() + "\""), nil
}

// UnmarshalJSON convert string into status
func (s *Status) UnmarshalJSON(b []byte) error {
	str := string(b)
	v, err := envexec.StringToStatus(str[1 : len(str)-1])
	if err != nil {
		return err
	}
	*s = Status(v)
	return nil
}

// Result defines single command result
type Result struct {
	Status     Status            `json:"status"`
	ExitStatus int               `json:"exitStatus"`
	Error      string            `json:"error,omitempty"`
	Time       uint64            `json:"time"`
	Memory     uint64            `json:"memory"`
	RunTime    uint64            `json:"runTime"`
	ProcPeak   uint64            `json:"procPeak,omitempty"`
	Files      map[string]string `json:"files,omitempty"`
	FileIDs    map[string]string `json:"fileIds,omitempty"`
	FileError  []FileError       `json:"fileError,omitempty"`

	files []string
	Buffs map[string][]byte `json:"-"`
}

func (r Result) String() string {
	type Result struct {
		Status     Status
		ExitStatus int
		Error      string
		Time       time.Duration
		RunTime    time.Duration
		ProcPeak   uint64
		Memory     envexec.Size
		Files      map[string]string
		FileIDs    map[string]string
		FileError  []FileError
	}
	d := Result{
		Status:     r.Status,
		ExitStatus: r.ExitStatus,
		Error:      r.Error,
		Time:       time.Duration(r.Time),
		RunTime:    time.Duration(r.RunTime),
		Memory:     envexec.Size(r.Memory),
		ProcPeak:   r.ProcPeak,
		Files:      make(map[string]string),
		FileIDs:    r.FileIDs,
		FileError:  r.FileError,
	}
	for k, v := range r.Files {
		d.Files[k] = "len:" + strconv.Itoa(len(v))
	}
	return fmt.Sprintf("%+v", d)
}

// Response defines worker response for single request
type Response struct {
	RequestID string   `json:"requestId"`
	Results   []Result `json:"results"`
	ErrorMsg  string   `json:"error,omitempty"`

	mmap bool
}

// Close need to be called when mmap specified to be true
func (r *Response) Close() {
	if !r.mmap {
		return
	}
	for _, res := range r.Results {
		res.Close()
	}
}

// Close need to be called when mmap specified to be true
func (r *Result) Close() {
	// remove temporary files
	for _, f := range r.files {
		os.Remove(f)
	}
	// remove potential mmap
	for _, b := range r.Buffs {
		releaseByte(b)
	}
}

// ConvertResponse converts
func ConvertResponse(r worker.Response, mmap bool) (ret Response, err error) {
	// in error case, release all resources
	defer func() {
		if err != nil {
			for _, r := range ret.Results {
				r.Close()
			}
			for _, r := range r.Results {
				for _, f := range r.Files {
					f.Close()
					os.Remove(f.Name())
				}
			}
		}
		// if no mmap required, close all files
		if !mmap {
			for _, r := range ret.Results {
				r.Close()
			}
		}
	}()

	ret = Response{
		RequestID: r.RequestID,
		Results:   make([]Result, 0, len(r.Results)),
		mmap:      mmap,
	}
	for _, r := range r.Results {
		res, err := convertResult(r, mmap)
		if err != nil {
			return ret, err
		}
		ret.Results = append(ret.Results, res)
	}
	if r.Error != nil {
		ret.ErrorMsg = r.Error.Error()
	}
	return ret, nil
}

// ConvertRequest converts json request into worker request
func ConvertRequest(r *Request, srcPrefix []string) (*worker.Request, error) {
	req := &worker.Request{
		RequestID:   r.RequestID,
		Cmd:         make([]worker.Cmd, 0, len(r.Cmd)),
		PipeMapping: make([]worker.PipeMap, 0, len(r.PipeMapping)),
	}
	for _, c := range r.Cmd {
		wc, err := convertCmd(c, srcPrefix)
		if err != nil {
			return nil, err
		}
		req.Cmd = append(req.Cmd, wc)
	}
	for _, p := range r.PipeMapping {
		req.PipeMapping = append(req.PipeMapping, convertPipe(p))
	}
	return req, nil
}

func convertResult(r worker.Result, mmap bool) (Result, error) {
	res := Result{
		Status:     Status(r.Status),
		ExitStatus: r.ExitStatus,
		Error:      r.Error,
		Time:       uint64(r.Time),
		RunTime:    uint64(r.RunTime),
		Memory:     uint64(r.Memory),
		ProcPeak:   r.ProcPeak,
		FileIDs:    r.FileIDs,
		FileError:  r.FileError,
	}
	if r.Files != nil {
		res.Files = make(map[string]string)
		res.Buffs = make(map[string][]byte)
		for k, f := range r.Files {
			b, err := fileToByte(f, mmap)
			if err != nil {
				return res, err
			}
			res.Files[k] = byteArrayToString(b)

			res.files = append(res.files, f.Name())
			res.Buffs[k] = b
		}
	}
	return res, nil
}

func convertPipe(p PipeMap) worker.PipeMap {
	return worker.PipeMap{
		In: worker.PipeIndex{
			Index: p.In.Index,
			Fd:    p.In.Fd,
		},
		Out: worker.PipeIndex{
			Index: p.Out.Index,
			Fd:    p.Out.Fd,
		},
		Proxy: p.Proxy,
		Name:  p.Name,
		Limit: worker.Size(p.Max),
	}
}

func convertCmd(c Cmd, srcPrefix []string) (worker.Cmd, error) {
	clockLimit := c.ClockLimit
	if c.RealCPULimit > 0 {
		clockLimit = c.RealCPULimit
	}
	w := worker.Cmd{
		Args:              c.Args,
		Env:               c.Env,
		Files:             make([]worker.CmdFile, 0, len(c.Files)),
		TTY:               c.TTY,
		CPULimit:          time.Duration(c.CPULimit),
		ClockLimit:        time.Duration(clockLimit),
		MemoryLimit:       envexec.Size(c.MemoryLimit),
		StackLimit:        envexec.Size(c.StackLimit),
		ProcLimit:         c.ProcLimit,
		CPURateLimit:      c.CPURateLimit,
		CPUSetLimit:       c.CPUSetLimit,
		DataSegmentLimit:  c.DataSegmentLimit || c.StrictMemoryLimit,
		AddressSpaceLimit: c.AddressSpaceLimit,
		CopyOut:           convertCopyOut(c.CopyOut),
		CopyOutCached:     convertCopyOut(c.CopyOutCached),
		CopyOutMax:        c.CopyOutMax,
		CopyOutDir:        c.CopyOutDir,
	}
	for _, f := range c.Files {
		cf, err := convertCmdFile(f, srcPrefix)
		if err != nil {
			return w, err
		}
		w.Files = append(w.Files, cf)
	}
	if c.CopyIn != nil {
		w.CopyIn = make(map[string]worker.CmdFile)
		w.Symlinks = make(map[string]string)
		for k, f := range c.CopyIn {
			if f.Symlink != nil {
				w.Symlinks[k] = *f.Symlink
				continue
			}
			cf, err := convertCmdFile(&f, srcPrefix)
			if err != nil {
				return w, err
			}
			w.CopyIn[k] = cf
		}
	}
	return w, nil
}

func convertCmdFile(f *CmdFile, srcPrefix []string) (worker.CmdFile, error) {
	switch {
	case f == nil:
		return nil, nil
	case f.Src != nil:
		if len(srcPrefix) != 0 {
			ok, err := CheckPathPrefixes(*f.Src, srcPrefix)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("file (%s) does not under (%s)", *f.Src, srcPrefix)
			}
		}
		return &worker.LocalFile{Src: *f.Src}, nil
	case f.Content != nil:
		return &worker.MemoryFile{Content: strToBytes(*f.Content)}, nil
	case f.FileID != nil:
		return &worker.CachedFile{FileID: *f.FileID}, nil
	case f.Max != nil && f.Name != nil:
		return &worker.Collector{Name: *f.Name, Max: envexec.Size(*f.Max), Pipe: f.Pipe}, nil
	default:
		return nil, fmt.Errorf("file is not valid for cmd")
	}
}

// CheckPathPrefixes ensure path is allowed by prefixes
func CheckPathPrefixes(path string, prefixes []string) (bool, error) {
	for _, p := range prefixes {
		ok, err := checkPathPrefix(path, p)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func checkPathPrefix(path, prefix string) (bool, error) {
	if filepath.IsAbs(path) {
		return strings.HasPrefix(filepath.Clean(path), prefix), nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return false, err
	}
	return strings.HasPrefix(filepath.Join(wd, path), prefix), nil
}

const optionalSuffix = "?"

func convertCopyOut(copyOut []string) []worker.CmdCopyOutFile {
	rt := make([]worker.CmdCopyOutFile, 0, len(copyOut))
	for _, n := range copyOut {
		if strings.HasSuffix(n, optionalSuffix) {
			rt = append(rt, worker.CmdCopyOutFile{
				Name:     strings.TrimSuffix(n, optionalSuffix),
				Optional: true,
			})
			continue
		}
		rt = append(rt, worker.CmdCopyOutFile{
			Name: n,
		})
	}
	return rt
}
