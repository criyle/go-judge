package grpcexecutor

import (
	"fmt"
	"os"

	"github.com/criyle/go-judge/filestore"
)

type fileStreamIn struct {
	name string
	r, w *os.File
}

func newFileStreamIn(name string) (*fileStreamIn, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	return &fileStreamIn{name: name, r: r, w: w}, nil
}

func (f *fileStreamIn) Name() string {
	return f.name
}

func (f *fileStreamIn) Write(b []byte) (int, error) {
	return f.w.Write(b)
}

func (f *fileStreamIn) EnvFile(fs filestore.FileStore) (interface{}, error) {
	return f.r, nil
}

func (f *fileStreamIn) String() string {
	return fmt.Sprintf("fileStreamIn:%s", f.name)
}

func (f *fileStreamIn) Close() error {
	f.r.Close()
	return f.w.Close()
}

type fileStreamOut struct {
	name string
	r, w *os.File
}

func newFileStreamOut(name string) (*fileStreamOut, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	return &fileStreamOut{name: name, r: r, w: w}, nil
}

func (f *fileStreamOut) Name() string {
	return f.name
}

func (f *fileStreamOut) Read(b []byte) (int, error) {
	return f.r.Read(b)
}

func (f *fileStreamOut) EnvFile(fs filestore.FileStore) (interface{}, error) {
	return f.w, nil
}

func (f *fileStreamOut) String() string {
	return fmt.Sprintf("fileStreamOut:%s", f.name)
}

func (f *fileStreamOut) Close() error {
	f.w.Close()
	return f.r.Close()
}
