package filestore

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"

	"github.com/criyle/go-judge/envexec"
)

const randIDLength = 12

// FileStore defines interface to store file
type FileStore interface {
	Add(name string, content []byte) (string, error) // Add creates a file with name & content to the storage, returns id
	Remove(string) bool                              // Remove deletes a file by id
	Get(string) (string, envexec.File)               // Get file by id, nil if not exists
	List() []string                                  // List return all file ids
}

func generateID() (string, error) {
	b := make([]byte, randIDLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if _, err := base32.NewEncoder(base32.StdEncoding, &buf).Write(b); err != nil {
		return "", err
	}
	return string(buf.Bytes()), nil
}
