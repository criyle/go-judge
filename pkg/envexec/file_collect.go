package envexec

import (
	"fmt"
	"io/ioutil"
	"os"

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

			// check size limit
			if c.CopyOutMax != 0 {
				stat, err := cf.Stat()
				if err != nil {
					return err
				}
				if s := stat.Size(); s > int64(c.CopyOutMax) {
					return fmt.Errorf("File %s have size %d exceeded the limit %d", n, s, c.CopyOutMax)
				}
			}

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
			return copyDir(m.WorkDir(), c.CopyOutDir)
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
