package filestore

import (
	"encoding/base32"
	"errors"
	"math/rand"
	"os"

	"github.com/criyle/go-judge/envexec"
)

const randIDLength = 5

var errUniqueIDNotGenerated = errors.New("unique id does not exists after tried 50 times")

// FileStore defines interface to store file
type FileStore interface {
	Add(name, path string) (string, error) // Add creates a file with path to the storage, returns id
	Remove(string) bool                    // Remove deletes a file by id
	Get(string) (string, envexec.File)     // Get file by id, nil if not exists
	List() map[string]string               // List return all file ids to original name
	New() (*os.File, error)                // Create a temporary file to the file store, can be added through Add to save it
}

func generateID() (string, error) {
	b := make([]byte, randIDLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.EncodeToString(b), nil
}
