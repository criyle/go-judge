package filestore

import (
	"bytes"
	"sync"

	"github.com/criyle/go-judge/envexec"
)

type fileMemoryStore struct {
	store map[string]fileMemory
	mu    sync.RWMutex
}

type fileMemory struct {
	name    string
	content []byte
}

// NewFileMemoryStore create new memory file store
func NewFileMemoryStore() FileStore {
	return &fileMemoryStore{
		store: make(map[string]fileMemory),
	}
}

func (s *fileMemoryStore) Add(name string, content []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		id  string
		err error
	)
	// generate until unique id (try maximun 50 times)
	for i := 0; i < 50; i++ {
		id, err = generateID()
		if err != nil {
			return "", err
		}
		if _, ok := s.store[id]; !ok {
			break
		}
	}
	if err != nil {
		return "", err
	}

	s.store[id] = fileMemory{name: name, content: content}
	return id, err
}

func (s *fileMemoryStore) Remove(fileID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.store[fileID]
	delete(s.store, fileID)
	return ok
}

func (s *fileMemoryStore) Get(fileID string) (string, envexec.File) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	f, ok := s.store[fileID]
	if !ok {
		return "", nil
	}
	return f.name, envexec.NewFileReader(bytes.NewReader(f.content), false)
}

func (s *fileMemoryStore) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b := make([]string, 0, len(s.store))
	for n := range s.store {
		b = append(b, n)
	}
	return b
}
