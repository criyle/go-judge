package model

import (
	"os"
	"syscall"
)

func fileToByte(f *os.File, mmap bool) ([]byte, error) {
	if mmap {
		return fileToByteMmap(f)
	}
	return fileToByteGeneric(f)
}

func fileToByteMmap(f *os.File) ([]byte, error) {
	defer f.Close()

	var s int64
	if fi, err := f.Stat(); err != nil {
		return nil, err
	} else {
		s = fi.Size()
	}
	if s == 0 {
		return []byte{}, nil
	}

	b, err := syscall.Mmap(int(f.Fd()), 0, int(s), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func releaseByte(b []byte) {
	if len(b) > 0 {
		_ = syscall.Munmap(b)
	}
}
