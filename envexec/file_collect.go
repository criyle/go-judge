package envexec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/criyle/go-sandbox/runner"
	"golang.org/x/sync/errgroup"
)

// copyOutAndCollect reads file and pipes in parallel from container
func copyOutAndCollect(m Environment, c *Cmd, ptc []pipeCollector) (map[string][]byte, error) {
	var (
		g errgroup.Group
		l sync.Mutex
	)
	rt := make(map[string][]byte)
	put := func(f []byte, n string) {
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
			var buf bytes.Buffer
			buf.Grow(int(s))

			// Ensure not copy over file size
			_, err = buf.ReadFrom(io.LimitReader(cf, s))
			if err != nil {
				return err
			}
			put(buf.Bytes(), n.Name)
			return nil
		})
	}

	// collect pipe
	for _, p := range ptc {
		p := p
		g.Go(func() error {
			<-p.done
			put(p.buffer.Bytes(), p.name)
			if int64(p.buffer.Len()) > int64(p.limit) {
				return runner.StatusOutputLimitExceeded
			}
			return nil
		})
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
