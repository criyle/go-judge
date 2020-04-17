package envexec

import (
	"io/ioutil"
	"os"
	"syscall"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-sandbox/runner"
	"golang.org/x/sync/errgroup"
)

// copyOutAndCollect reads file and pipes in parallel from container
func copyOutAndCollect(m Environment, c *Cmd, ptc []pipeCollector) (map[string]file.File, error) {
	var g errgroup.Group
	fc := make(chan file.File, len(ptc)+len(c.CopyOut))

	// copy out
	for _, n := range c.CopyOut {
		n := n
		g.Go(func() error {
			cf, err := m.Open(n, os.O_RDONLY, 0777)
			if err != nil {
				return err
			}
			defer cf.Close()

			c, err := ioutil.ReadAll(cf)
			if err != nil {
				return err
			}
			fc <- file.NewMemFile(n, c)
			return nil
		})
	}

	// collect pipe
	for _, p := range ptc {
		p := p
		g.Go(func() error {
			<-p.buff.Done
			if int64(p.buff.Buffer.Len()) > p.buff.Max {
				return runner.StatusOutputLimitExceeded
			}
			fc <- file.NewMemFile(p.name, p.buff.Buffer.Bytes())
			return nil
		})
	}

	// copy out dir
	if c.CopyOutDir != "" {
		g.Go(func() error {
			// make sure dir exists
			os.MkdirAll(c.CopyOutDir, 0777)
			newDir, err := os.Open(c.CopyOutDir)
			if err != nil {
				return err
			}
			defer newDir.Close()

			dir := m.WorkDir()
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
		})
	}

	var err error
	go func() {
		err = g.Wait()
		close(fc)
	}()

	rt := make(map[string]file.File)
	for f := range fc {
		rt[f.Name()] = f
	}
	return rt, err
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
