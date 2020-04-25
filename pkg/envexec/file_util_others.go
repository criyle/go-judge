// +build !linux

package envexec

import (
	"io"
	"os"
	"path"
)

func copyDir(src *os.File, dst string) error {
	// make sure dir exists
	os.MkdirAll(dst, 0777)
	newDir, err := os.Open(dst)
	if err != nil {
		return err
	}
	defer newDir.Close()

	dir := src
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
	t, err := os.OpenFile(path.Join(dst, name), os.O_WRONLY, 0777)
	if err != nil {
		return err
	}
	_, err = io.Copy(t, s)
	if err != nil {
		return err
	}
	return nil
}