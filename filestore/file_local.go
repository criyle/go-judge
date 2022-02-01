package filestore

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/criyle/go-judge/envexec"
)

type fileLocalStore struct {
	dir  string            // directory to store file
	name map[string]string // id to name mapping if exists
	mu   sync.RWMutex
}

// NewFileLocalStore create new local file store
func NewFileLocalStore(dir string) FileStore {
	return &fileLocalStore{
		dir:  filepath.Clean(dir),
		name: make(map[string]string),
	}
}

func (s *fileLocalStore) Add(name, path string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.dir == filepath.Dir(path) {
		id := filepath.Base(path)
		s.name[id] = name
		return id, nil
	}
	return "", fmt.Errorf("add: %s does not have prefix %s", path, s.dir)
}

func (s *fileLocalStore) Get(id string) (string, envexec.File) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p := path.Join(s.dir, id)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return "", nil
	}
	name, ok := s.name[id]
	if !ok {
		name = id
	}
	return name, envexec.NewFileInput(p)
}

func (s *fileLocalStore) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.name, id)
	p := path.Join(s.dir, id)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return false
	}
	os.Remove(p)
	return true
}

func (s *fileLocalStore) List() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fi, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}

	names := make(map[string]string, len(fi))
	for _, f := range fi {
		names[f.Name()] = s.name[f.Name()]
	}
	return names
}

func (s *fileLocalStore) New() (*os.File, error) {
	for range [50]struct{}{} {
		id, err := generateID()
		if err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path.Join(s.dir, id), os.O_CREATE|os.O_RDWR|os.O_EXCL, 0644)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	return nil, errUniqueIDNotGenerated
}
