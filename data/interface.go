package data

import "github.com/criyle/go-judge/file"

// Builder return a file collection by given id
type Builder interface {
	New(id string) (Data, error)
}

// Data defines interface to download file from website
type Data interface {
	// ID returns the underlying id
	ID() string
	// Files returns map: file name -> file
	Files() map[string]file.File
}
