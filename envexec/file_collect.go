package envexec

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/criyle/go-sandbox/runner"
	"golang.org/x/sync/errgroup"
)

// copyOutAndCollect reads file and pipes in parallel from container
func copyOutAndCollect(m Environment, c *Cmd, ptc []pipeCollector, newStoreFile NewStoreFile) (map[string]*os.File, []FileError, error) {
	var (
		g         errgroup.Group
		l, le     sync.Mutex
		fileError []FileError
	)
	rt := make(map[string]*os.File)
	put := func(f *os.File, n string) {
		l.Lock()
		defer l.Unlock()
		rt[n] = f
	}
	addError := func(e FileError) {
		le.Lock()
		defer le.Unlock()
		fileError = append(fileError, e)
	}

	// copy out files
	copyOutFiles(&g, m, c, newStoreFile, put, addError)

	// collect pipes
	for _, p := range ptc {
		p := p
		g.Go(func() error {
			return collectPipe(p, newStoreFile, put, addError)
		})
	}

	// copy out dir, disable for safety
	// if c.CopyOutDir != "" {
	// 	g.Go(func() error {
	// 		return copyDir(m.WorkDir(), c.CopyOutDir)
	// 	})
	// }

	err := g.Wait()
	return rt, fileError, err
}

func copyOutFiles(g *errgroup.Group, m Environment, c *Cmd, newStoreFile NewStoreFile, put func(*os.File, string), addError func(FileError)) error {
	cmds := make([]OpenParam, 0, len(c.CopyOut))
	for _, n := range c.CopyOut {
		cmds = append(cmds, OpenParam{
			Path: n.Name,
			Flag: os.O_RDONLY,
			Perm: 0777,
		})
	}

	results, err := m.Open(cmds)
	if err != nil {
		return fmt.Errorf("copyout: batch open failed: %w", err)
	}

	for i, res := range results {
		i, res := i, res
		n := c.CopyOut[i]

		if res.Err != nil {
			if errors.Is(res.Err, os.ErrNotExist) && n.Optional {
				continue
			}
			addError(FileError{
				Name:    n.Name,
				Type:    ErrCopyOutOpen,
				Message: res.Err.Error(),
			})
			continue
		}

		// Parallelize the I/O heavy part (stat and data streaming)
		g.Go(func() error {
			// Ensure the file received from the container is always closed
			defer res.File.Close()
			return copyOutFileStream(res.File, c, n, newStoreFile, put, addError)
		})
	}

	return g.Wait()
}

func copyOutFileStream(
	cf *os.File, // The file already opened via batch IPC
	c *Cmd,
	n CmdCopyOutFile,
	newStoreFile NewStoreFile,
	put func(*os.File, string),
	addError func(FileError),
) (err error) {
	t := ErrCopyOutOpen
	defer func() {
		if err != nil {
			addError(FileError{
				Name:    n.Name,
				Type:    t,
				Message: err.Error(),
			})
		}
	}()

	stat, err := cf.Stat()
	if err != nil {
		return fmt.Errorf("copyout: stat %q: %w", n.Name, err)
	}

	// Check regular file
	if stat.Mode()&os.ModeType != 0 {
		t = ErrCopyOutNotRegularFile
		return fmt.Errorf("copyout: %q is not a regular file: %v", n.Name, stat.Mode())
	}

	// Check size limit
	s := stat.Size()
	limitExceeded := false
	if c.CopyOutMax > 0 && s > int64(c.CopyOutMax) {
		if !c.CopyOutTruncate {
			t = ErrCopyOutSizeExceeded
			return fmt.Errorf("copyout: %q size (%d) exceeds limit (%d)", n.Name, s, c.CopyOutMax)
		}
		s = int64(c.CopyOutMax)
		limitExceeded = true
	}

	// Create store file
	buf, err := newStoreFile()
	if err != nil {
		t = ErrCopyOutCreateFile
		return fmt.Errorf("copyout: failed to create store file for %q: %w", n.Name, err)
	}

	// Stream content
	written, err := buf.ReadFrom(io.LimitReader(cf, s))
	if err != nil {
		t = ErrCopyOutCopyContent
		buf.Close()
		return fmt.Errorf("copyout: failed to copy content for %q: %w", n.Name, err)
	}
	put(buf, n.Name)

	// If we truncated or if the file grew during read beyond the limit
	if limitExceeded || (c.CopyOutMax > 0 && written > int64(c.CopyOutMax)) {
		t = ErrCopyOutSizeExceeded
		return fmt.Errorf("copyout: %q size exceeds limit (%d)", n.Name, c.CopyOutMax)
	}
	return nil
}

func collectPipe(
	p pipeCollector,
	newStoreFile NewStoreFile,
	put func(*os.File, string),
	addError func(FileError),
) (err error) {
	errType := ErrCopyOutOpen
	defer func() {
		if err != nil {
			addError(FileError{
				Name:    p.name,
				Type:    errType,
				Message: err.Error(),
			})
		}
	}()
	<-p.done
	if p.storage {
		put(p.buffer, p.name)
		if fi, err := p.buffer.Stat(); err == nil && fi.Size() > int64(p.limit) {
			p.buffer.Truncate(int64(p.limit) + 1)
			errType = ErrCollectSizeExceeded
			return runner.StatusOutputLimitExceeded
		}
	} else {
		defer p.buffer.Close()
		buf, err := newStoreFile()
		if err != nil {
			errType = ErrCopyOutCreateFile
			return fmt.Errorf("collect: failed to create store file for %q: %w", p.name, err)
		}
		// Ensure not copy over file size
		_, err = buf.ReadFrom(io.LimitReader(p.buffer, int64(p.limit)+1))
		if err != nil {
			errType = ErrCopyOutCopyContent
			buf.Close()
			return fmt.Errorf("collect: failed to copy content for %q: %w", p.name, err)
		}
		put(buf, p.name)
		if fi, err := p.buffer.Stat(); err == nil && fi.Size() > int64(p.limit) {
			errType = ErrCollectSizeExceeded
			return runner.StatusOutputLimitExceeded
		}
	}
	return nil
}
