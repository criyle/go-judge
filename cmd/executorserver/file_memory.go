package main

import (
	"sync"

	"github.com/criyle/go-judge/file"
)

var _ fileStore = &fileMemoryStore{}

type fileMemoryStore struct {
	store map[string]file.File
	mu    sync.RWMutex
}

func newFileMemoryStore() *fileMemoryStore {
	return &fileMemoryStore{
		store: make(map[string]file.File),
	}
}

func (s *fileMemoryStore) Add(fileName string, content []byte) (string, error) {
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

	s.store[id] = file.NewMemFile(fileName, content)
	return id, err
}

func (s *fileMemoryStore) Remove(fileID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.store[fileID]
	delete(s.store, fileID)
	return ok
}

func (s *fileMemoryStore) Get(fileID string) file.File {
	s.mu.RLock()
	defer s.mu.RUnlock()

	f := s.store[fileID]
	return f
}

func (s *fileMemoryStore) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var b []string
	for n := range s.store {
		b = append(b, n)
	}
	return b
}
