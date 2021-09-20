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
func copyOutAndCollect(m Environment, c *Cmd, ptc []pipeCollector, newStoreFile NewStoreFile) (map[string]*os.File, error) {
	var (
		g errgroup.Group
		l sync.Mutex
	)
	rt := make(map[string]*os.File)
	put := func(f *os.File, n string) {
		l.Lock()
		defer l.Unlock()
		rt[n] = f
	}

	// copy out
	for _, n := range c.CopyOut {
		n := n
		g.Go(func() error {
			cf, err := m.Open(n.Name, os.O_RDONLY, 0777)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) && n.Optional {
					return nil
				}
				return err
			}
			defer cf.Close()

			stat, err := cf.Stat()
			if err != nil {
				return err
			}
			// check regular file
			if stat.Mode()&os.ModeType != 0 {
				return fmt.Errorf("%s: not a regular file %d", n.Name, stat.Mode()&os.ModeType)
			}
			// check size limit
			s := stat.Size()
			if c.CopyOutMax > 0 && s > int64(c.CopyOutMax) {
				return fmt.Errorf("%s: size (%d) exceeded the limit (%d)", n.Name, s, c.CopyOutMax)
			}
			// create store file
			buf, err := newStoreFile()
			if err != nil {
				return fmt.Errorf("%s: failed to create store file %v", n.Name, err)
			}

			// Ensure not copy over file size
			_, err = buf.ReadFrom(io.LimitReader(cf, s))
			if err != nil {
				buf.Close()
				return err
			}
			put(buf, n.Name)
			return nil
		})
	}

	// collect pipe
	for _, p := range ptc {
		p := p
		g.Go(func() error {
			<-p.done
			put(p.buffer, p.name)
			if fi, err := p.buffer.Stat(); err == nil && fi.Size() > int64(p.limit) {
				return runner.StatusOutputLimitExceeded
			}
			return nil
		})
	}

	// collect container collector
	ct := make(map[string]bool)
	for _, f := range c.Files {
		if t, ok := f.(*FileCollector); ok {
			if t.Pipe || ct[t.Name] {
				continue
			}
			ct[t.Name] = true

			g.Go(func() error {
				cf, err := m.Open(t.Name, os.O_RDONLY, 0777)
				if err != nil {
					return err
				}
				defer cf.Close()

				stat, err := cf.Stat()
				if err != nil {
					return err
				}
				// check regular file
				if stat.Mode()&os.ModeType != 0 {
					return fmt.Errorf("%s: not a regular file %d", t.Name, stat.Mode()&os.ModeType)
				}

				// create store file
				buf, err := newStoreFile()
				if err != nil {
					return fmt.Errorf("%s: failed to create store file %v", t.Name, err)
				}

				// Ensure not copy over file size
				_, err = buf.ReadFrom(io.LimitReader(cf, int64(t.Limit)+1))
				if err != nil {
					buf.Close()
					return err
				}
				put(buf, t.Name)

				// check size limit
				s := stat.Size()
				if s > int64(t.Limit) {
					return runner.StatusOutputLimitExceeded
				}
				return nil
			})
		}
	}

	// copy out dir
	if c.CopyOutDir != "" {
		g.Go(func() error {
			return copyDir(m.WorkDir(), c.CopyOutDir)
		})
	}

	err := g.Wait()
	return rt, err
}
