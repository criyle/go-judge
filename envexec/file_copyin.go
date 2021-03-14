package envexec

import (
	"fmt"
	"os"

	"golang.org/x/sync/errgroup"
)

// copyIn copied file from host to container in parallel
func copyIn(m Environment, copyIn map[string]File) error {
	var g errgroup.Group
	for n, f := range copyIn {
		n, f := n, f
		g.Go(func() error {
			hf, err := FileToReader(f)
			if err != nil {
				return fmt.Errorf("failed to copyIn %v", err)
			}
			defer hf.Close()

			cf, err := m.Open(n, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0777)
			if err != nil {
				return err
			}
			defer cf.Close()

			_, err = cf.ReadFrom(hf)
			if err != nil {
				return err
			}
			return nil
		})
	}
	return g.Wait()
}
