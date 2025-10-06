package envexec

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/criyle/go-sandbox/runner"
)

// Size represent data size in bytes
type Size = runner.Size

// RunnerResult represent process finish result
type RunnerResult = runner.Result

// Cmd defines instruction to run a program in container environment
type Cmd struct {
	Environment Environment

	// file contents to copyin before exec
	CopyIn map[string]File

	// symbolic link to be created before exec
	SymLinks map[string]string

	// exec argument, environment
	Args []string
	Env  []string

	// Files for the executing command
	Files []File
	TTY   bool // use pty as input / output

	// resource limits
	TimeLimit        time.Duration
	MemoryLimit      Size
	StackLimit       Size
	ExtraMemoryLimit Size
	OutputLimit      Size
	ProcLimit        uint64
	OpenFileLimit    uint64
	CPURateLimit     uint64
	CPUSetLimit      string

	// Waiter is called after cmd starts and it should return
	// once time limit exceeded.
	// return true to as TLE and false as normal exits (context finished)
	Waiter func(context.Context, Process) bool

	// file names to copyout after exec
	CopyOut         []CmdCopyOutFile
	CopyOutMax      Size // file size limit
	CopyOutTruncate bool

	// CopyOutDir specifies a dir to dump all /w content
	CopyOutDir string

	// additional memory option
	AddressSpaceLimit bool
	DataSegmentLimit  bool
}

// CmdCopyOutFile defines the file to be copy out after cmd execution
type CmdCopyOutFile struct {
	Name     string // Name is the file out to copyOut
	Optional bool   // Optional ignores the file if not exists
}

// Result defines the running result for single Cmd
type Result struct {
	Status Status

	ExitStatus int

	Error string // error

	Time     time.Duration
	RunTime  time.Duration
	Memory   Size   // byte
	ProcPeak uint64 // maximum processes ever running

	// Files stores copy out files
	Files map[string]*os.File

	// FileError stores file errors details
	FileError []FileError
}

// FileErrorType defines the location that file operation fails
type FileErrorType int

// FileError enums
const (
	ErrCopyInOpenFile FileErrorType = iota
	ErrCopyInCreateDir
	ErrCopyInCreateFile
	ErrCopyInCopyContent
	ErrCopyOutOpen
	ErrCopyOutNotRegularFile
	ErrCopyOutSizeExceeded
	ErrCopyOutCreateFile
	ErrCopyOutCopyContent
	ErrCollectSizeExceeded
	ErrSymlink
)

// FileError defines the location, file name and the detailed message for a failed file operation
type FileError struct {
	Name    string        `json:"name"`
	Type    FileErrorType `json:"type"`
	Message string        `json:"message,omitempty"`
}

var fileErrorString = []string{
	"CopyInOpenFile",
	"CopyInCreateDir",
	"CopyInCreateFile",
	"CopyInCopyContent",
	"CopyOutOpen",
	"CopyOutNotRegularFile",
	"CopyOutSizeExceeded",
	"CopyOutCreateFile",
	"CopyOutCopyContent",
	"CollectSizeExceeded",
}

var fileErrorStringReverse = make(map[string]FileErrorType)

func (t FileErrorType) String() string {
	v := int(t)
	if v >= 0 && v < len(fileErrorString) {
		return fileErrorString[v]
	}
	return ""
}

// MarshalJSON encodes file error into json string
func (t FileErrorType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}

// UnmarshalJSON decodes file error from json string
func (t *FileErrorType) UnmarshalJSON(b []byte) error {
	str := string(b)
	v, ok := fileErrorStringReverse[str]
	if !ok {
		return fmt.Errorf("%s is not file error type", str)
	}
	*t = v
	return nil
}

func init() {
	for i, v := range fileErrorString {
		fileErrorStringReverse[`"`+v+`"`] = FileErrorType(i)
	}
}
