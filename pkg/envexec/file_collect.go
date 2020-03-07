package envexec

import (
	"io/ioutil"
	"os"
	"sync"
	"syscall"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/runner"
)

func copyOutAndCollect(m container.Environment, c *Cmd, ptc []pipeCollector) (map[string]file.File, error) {
	// wait to complete
	var wg sync.WaitGroup

	fc := make(chan file.File, len(ptc)+len(c.CopyOut))
	errC := make(chan error, 1) // collect only 1 error

	var (
		cFiles []*os.File
		err    error
	)
	if len(c.CopyOut) > 0 {
		// prepare open param
		openCmd := make([]container.OpenCmd, 0, len(c.CopyOut))
		for _, n := range c.CopyOut {
			openCmd = append(openCmd, container.OpenCmd{
				Path: n,
				Flag: os.O_RDONLY,
			})
		}

		// open all
		cFiles, err = m.Open(openCmd)
	}

	// copy out
	wg.Add(len(cFiles))
	for i := range cFiles {
		go func(cFile *os.File, n string) {
			defer wg.Done()
			defer cFile.Close()
			c, err := ioutil.ReadAll(cFile)
			if err != nil {
				writeErrorC(errC, err)
				return
			}
			fc <- file.NewMemFile(n, c)
		}(cFiles[i], c.CopyOut[i])
	}

	// collect pipe
	wg.Add(len(ptc))
	for _, p := range ptc {
		go func(p pipeCollector) {
			defer wg.Done()
			<-p.buff.Done
			if int64(p.buff.Buffer.Len()) > p.buff.Max {
				writeErrorC(errC, runner.StatusOutputLimitExceeded)
			}
			fc <- file.NewMemFile(p.name, p.buff.Buffer.Bytes())
		}(p)
	}

	// copy out dir
	if c.CopyOutDir != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// open container /w
			openCmd := []container.OpenCmd{{
				Path: "/w",
				Flag: os.O_RDONLY,
			}}
			dir, err := m.Open(openCmd)
			if err != nil {
				writeErrorC(errC, err)
				return
			}
			defer dir[0].Close()

			// make sure dir exists
			os.MkdirAll(c.CopyOutDir, 0777)
			newDir, err := os.Open(c.CopyOutDir)
			if err != nil {
				writeErrorC(errC, err)
				return
			}
			defer newDir.Close()

			names, err := dir[0].Readdirnames(-1)
			if err != nil {
				writeErrorC(errC, err)
				return
			}
			for _, n := range names {
				if err := copyFileDir(int(dir[0].Fd()), int(newDir.Fd()), n); err != nil {
					writeErrorC(errC, err)
					return
				}
				// Link at do not cross device copy, sad..
				// if err := unix.Linkat(int(dir[0].Fd()), n, int(newDir.Fd()), n, 0); err != nil {
				// 	writeErrorC(errC, err)
				// 	return
				// }
			}
		}()
	}

	// wait to finish
	wg.Wait()

	// collect to map
	close(fc)
	rt := make(map[string]file.File)
	for f := range fc {
		name := f.Name()
		rt[name] = f
	}

	if err != nil {
		return rt, err
	}

	// check error
	select {
	case err := <-errC:
		return rt, err
	default:
	}

	return rt, nil
}

func copyFileDir(srcDirFd, dstDirFd int, name string) error {
	// open the source file
	fd, err := syscall.Openat(srcDirFd, name, syscall.O_RDONLY, 0777)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)

	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		return err
	}
	// open the dst file
	dstFd, err := syscall.Openat(dstDirFd, name, syscall.O_WRONLY|syscall.O_CREAT, 0777)
	if err != nil {
		return err
	}
	defer syscall.Close(dstFd)

	// send file
	if _, err := syscall.Sendfile(dstFd, fd, nil, int(st.Size)); err != nil {
		return err
	}
	return nil
}
