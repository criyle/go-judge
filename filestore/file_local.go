package filestore

import (
	"errors"
	"os"
	"path"
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
		dir:  dir,
		name: make(map[string]string),
	}
}

func (s *fileLocalStore) Add(name string, content []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, err := generateUniqueID(func(id string) (bool, error) {
		_, err := os.Stat(path.Join(s.dir, id))
		switch {
		case errors.Is(err, os.ErrNotExist):
			return false, nil
		case err == nil:
			return true, nil
		default:
			return false, err
		}
	})
	if err != nil {
		return "", err
	}
	err = os.WriteFile(path.Join(s.dir, id), content, 0644)
	if err != nil {
		return "", err
	}
	s.name[id] = name
	return id, err
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
