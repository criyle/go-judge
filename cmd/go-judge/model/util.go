package model

import (
	"io"
	"os"
	"unsafe"
)

func fileToByteGeneric(f *os.File) ([]byte, error) {
	defer f.Close()

	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	s := fi.Size()

	c := make([]byte, s)
	if _, err := io.ReadFull(f, c); err != nil {
		return nil, err
	}
	return c, nil
}

func strToBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
