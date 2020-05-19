package filestore

import (
	"io/ioutil"
	"os"
	"path"
	"sync"

	"github.com/criyle/go-judge/file"
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
		if _, err := os.Stat(path.Join(s.dir, id)); err == nil {
			break
		}
	}
	if err != nil {
		return "", err
	}
	err = ioutil.WriteFile(path.Join(s.dir, id), content, 0644)
	if err != nil {
		return "", err
	}
	s.name[id] = name
	return id, err
}

func (s *fileLocalStore) Get(id string) file.File {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p := path.Join(s.dir, id)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil
	}
	name, ok := s.name[id]
	if !ok {
		name = id
	}
	return file.NewLocalFile(name, p)
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

func (s *fileLocalStore) List() []string {
	var names []string
	fi, err := ioutil.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	for _, f := range fi {
		names = append(names, f.Name())
	}
	return names
}
