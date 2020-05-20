package worker

import (
	"fmt"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/pkg/envexec"
)

// CmdFile defines file used in the cmd
type CmdFile interface {
	// EnvFile prepares file for envexec file
	EnvFile(fs filestore.FileStore) (interface{}, error)
}

var (
	_ CmdFile = &LocalFile{}
	_ CmdFile = &MemoryFile{}
	_ CmdFile = &CachedFile{}
	_ CmdFile = &PipeCollector{}
)

// LocalFile defines file stores on the local file system
type LocalFile struct {
	Src string
}

// EnvFile prepares file for envexec file
func (f *LocalFile) EnvFile(fs filestore.FileStore) (interface{}, error) {
	return file.NewLocalFile(f.Src, f.Src), nil
}

// MemoryFile defines file stores in the memory
type MemoryFile struct {
	Content []byte
}

// EnvFile prepares file for envexec file
func (f *MemoryFile) EnvFile(fs filestore.FileStore) (interface{}, error) {
	return file.NewMemFile("", f.Content), nil
}

// CachedFile defines file cached in the file store
type CachedFile struct {
	FileID string
}

// EnvFile prepares file for envexec file
func (f *CachedFile) EnvFile(fs filestore.FileStore) (interface{}, error) {
	fd := fs.Get(f.FileID)
	if fd == nil {
		return nil, fmt.Errorf("file not exists with id %v", f.FileID)
	}
	return fd, nil
}

// PipeCollector defines on the output (stdout / stderr) to be collected over pipe
type PipeCollector struct {
	Name string // pseudo name generated into copyOut
	Max  int64  // max size to be collected
}

// EnvFile prepares file for envexec file
func (f *PipeCollector) EnvFile(fs filestore.FileStore) (interface{}, error) {
	return envexec.PipeCollector{Name: f.Name, SizeLimit: f.Max}, nil
}
