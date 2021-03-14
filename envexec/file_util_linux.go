package envexec

import (
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"syscall"

	"github.com/criyle/go-sandbox/pkg/memfd"
)

const memfdName = "input"

var enableMemFd int32

func readerToFile(reader io.Reader) (*os.File, error) {
	if atomic.LoadInt32(&enableMemFd) == 0 {
		f, err := memfd.DupToMemfd(memfdName, reader)
		if err == nil {
			return f, err
		}
		atomic.StoreInt32(&enableMemFd, 1)
	}
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

	dir := src
	names, err := dir.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, n := range names {
		if err := copyFileDir(int(dir.Fd()), int(newDir.Fd()), n); err != nil {
			return err
		}
	}
	return nil
}

func copyFileDir(srcDirFd, dstDirFd int, name string) error {
	// open the source file
	fd, err := syscall.Openat(srcDirFd, name, syscall.O_CLOEXEC|syscall.O_RDONLY, 0777)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)

	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&syscall.S_IFREG == 0 {
		return fmt.Errorf("%s is not a regular file", name)
	}

	// open the dst file
	dstFd, err := syscall.Openat(dstDirFd, name, syscall.O_CLOEXEC|syscall.O_WRONLY|syscall.O_CREAT|syscall.O_TRUNC, 0777)
	if err != nil {
		return err
	}
	defer syscall.Close(dstFd)

	// send file, ignore error for now if it is dir
	if _, err := syscall.Sendfile(dstFd, fd, nil, int(st.Size)); err != nil && err != syscall.EINVAL {
		return err
	}
	return nil
}
