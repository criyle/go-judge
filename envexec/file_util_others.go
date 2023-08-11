//go:build !linux

package envexec

import (
	"fmt"
	"io"
	"os"
	"path"
)

func readerToFile(reader io.Reader) (*os.File, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	go func() {
		defer w.Close()
		w.ReadFrom(reader)
	}()
	return r, nil
}

func copyDir(src *os.File, dst string) error {
	// make sure dir exists
	os.MkdirAll(dst, 0777)
	newDir, err := os.Open(dst)
	if err != nil {
		return err
	}
	defer newDir.Close()

	dir, err := os.Open(src.Name())
	if err != nil {
		return err
	}
	defer dir.Close()

	names, err := dir.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, n := range names {
		if copyDirFile(src.Name(), dst, n); err != nil {
			return err
		}
	}
	return nil
}

func copyDirFile(src, dst, name string) error {
	s, err := os.Open(path.Join(src, name))
	if err != nil {
		return err
	}
	defer s.Close()

	t, err := os.OpenFile(path.Join(dst, name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
	if err != nil {
		return err
	}
	defer t.Close()

	stat, err := t.Stat()
	if err != nil {
		return err
	}

	// check regular file
	if stat.Mode()&os.ModeType != 0 {
		return fmt.Errorf("File(%s) is not a regular file", name)
	}

	_, err = t.ReadFrom(s)
	if err != nil {
		return err
	}
	return nil
}
