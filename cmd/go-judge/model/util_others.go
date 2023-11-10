//go:build !linux

package model

import "os"

func fileToByte(f *os.File, mmap bool) ([]byte, error) {
	return fileToByteGeneric(f)
}

func releaseByte(b []byte) {
}
