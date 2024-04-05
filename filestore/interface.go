package filestore

import (
	"encoding/base32"
	"errors"
	"math/rand/v2"
	"os"

	"github.com/criyle/go-judge/envexec"
)

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
	const randIDLength = 5
	b := make([]byte, randIDLength)
	r := rand.Int64N(1 << 40)
	b[0] = byte(r)
	b[1] = byte(r >> 8)
	b[2] = byte(r >> 16)
	b[3] = byte(r >> 24)
	b[4] = byte(r >> 32)
	return base32.StdEncoding.EncodeToString(b), nil
}
