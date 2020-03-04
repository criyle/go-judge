package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"sync"
)

const randIDLength = 12

type fileData struct {
	Content  []byte
	FileName string
}

var (
	fileStore     map[string]*fileData
	fileStoreLock sync.RWMutex
)

func init() {
	fileStore = make(map[string]*fileData)
}

func generateID() (string, error) {
	b := make([]byte, randIDLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if _, err := base64.NewEncoder(base64.StdEncoding, &buf).Write(b); err != nil {
		return "", err
	}
	return string(buf.Bytes()), nil
}

func addFile(content []byte, fileName string) (string, error) {
	fileStoreLock.Lock()
	defer fileStoreLock.Unlock()

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
		if _, ok := fileStore[id]; !ok {
			break
		}
	}
	if err != nil {
		return "", err
	}

	fileStore[id] = &fileData{
		Content:  content,
		FileName: fileName,
	}
	return id, err
}

func removeFile(fileID string) bool {
	fileStoreLock.Lock()
	defer fileStoreLock.Unlock()

	_, ok := fileStore[fileID]
	delete(fileStore, fileID)
	return ok
}

func getFile(fileID string) (*fileData, bool) {
	fileStoreLock.RLock()
	defer fileStoreLock.RUnlock()

	f, ok := fileStore[fileID]
	return f, ok
}

func getAllFileID() []string {
	fileStoreLock.RLock()
	defer fileStoreLock.RUnlock()

	var b []string
	for n := range fileStore {
		b = append(b, n)
	}
	return b
}
