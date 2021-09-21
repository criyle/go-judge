//go:build !linux
// +build !linux

package model

import (
	"io"
	"os"
)

func fileToByte(f *os.File, mmap bool) ([]byte, error) {
	return fileToByteGeneric(f)
}

func releaseByte(b []byte) {
}
