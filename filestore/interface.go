package filestore

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"errors"

	"github.com/criyle/go-judge/envexec"
)

const randIDLength = 12

var errUniqueIDNotGenerated = errors.New("Unique id does not exists after tried 50 times")

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

func generateUniqueID(isExists func(string) (bool, error)) (string, error) {
	for range [50]struct{}{} {
		id, err := generateID()
		if err != nil {
			return "", err
		}
		exists, err := isExists(id)
		if err != nil {
			return "", err
		}
		if !exists {
			return id, nil
		}
	}
	return "", errUniqueIDNotGenerated
}
