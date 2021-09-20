package worker

import (
	"bytes"
	"fmt"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/filestore"
)

// CmdFile defines file used in the cmd
type CmdFile interface {
	// EnvFile prepares file for envexec file
	EnvFile(fs filestore.FileStore) (envexec.File, error)
	// Stringer to print debug information
	String() string
}

var (
	_ CmdFile = &LocalFile{}
	_ CmdFile = &MemoryFile{}
	_ CmdFile = &CachedFile{}
	_ CmdFile = &Collector{}
)

// LocalFile defines file stores on the local file system
type LocalFile struct {
	Src string
}

// EnvFile prepares file for envexec file
func (f *LocalFile) EnvFile(fs filestore.FileStore) (envexec.File, error) {
	return envexec.NewFileInput(f.Src), nil
}

func (f *LocalFile) String() string {
	return fmt.Sprintf("local:%s", f.Src)
}

// MemoryFile defines file stores in the memory
type MemoryFile struct {
	Content []byte
}

// EnvFile prepares file for envexec file
func (f *MemoryFile) EnvFile(fs filestore.FileStore) (envexec.File, error) {
	return envexec.NewFileReader(bytes.NewReader(f.Content), false), nil
}

func (f *MemoryFile) String() string {
	return fmt.Sprintf("memory:(len:%d)", len(f.Content))
}

// CachedFile defines file cached in the file store
type CachedFile struct {
	FileID string
}

// EnvFile prepares file for envexec file
func (f *CachedFile) EnvFile(fs filestore.FileStore) (envexec.File, error) {
	_, fd := fs.Get(f.FileID)
	if fd == nil {
		return nil, fmt.Errorf("file not exists with id %v", f.FileID)
	}
	return fd, nil
}

func (f *CachedFile) String() string {
	return fmt.Sprintf("cached:(fileId:%s)", f.FileID)
}

// Collector defines on the output (stdout / stderr) to be collected over pipe
type Collector struct {
	Name string       // pseudo name generated into copyOut
	Max  envexec.Size // max size to be collected
	Pipe bool
}

// EnvFile prepares file for envexec file
func (f *Collector) EnvFile(fs filestore.FileStore) (envexec.File, error) {
	return envexec.NewFileCollector(f.Name, f.Max, f.Pipe), nil
}

func (f *Collector) String() string {
	return fmt.Sprintf("collector:(name:%s,max:%d,pipe:%v)", f.Name, f.Max, f.Pipe)
}
