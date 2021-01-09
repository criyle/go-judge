package envexec

import (
	"os"

	"github.com/criyle/go-judge/file"
	"golang.org/x/sync/errgroup"
)

// copyIn copied file from host to container in parallel
func copyIn(m Environment, copyIn map[string]file.File) error {
	var g errgroup.Group
	for n, f := range copyIn {
		n, f := n, f
		g.Go(func() error {
			cf, err := m.Open(n, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0777)
			if err != nil {
				return err
			}
			defer cf.Close()

			hf, err := f.Reader()
			if err != nil {
				return err
			}
			defer hf.Close()

			_, err = cf.ReadFrom(hf)
			if err != nil {
				return err
			}
			return nil
		})
	}
	return g.Wait()
}
