package envexec

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sync/errgroup"
)

// copyIn copied file from host to container in parallel
func copyIn(m Environment, copyIn map[string]File) ([]FileError, error) {
	var (
		g         errgroup.Group
		fileError []FileError
		l         sync.Mutex
	)
	addError := func(e FileError) {
		l.Lock()
		defer l.Unlock()
		fileError = append(fileError, e)
	}
	for n, f := range copyIn {
		n, f := n, f
		g.Go(func() (err error) {
			t := ErrCopyInOpenFile
			defer func() {
				if err != nil {
					addError(FileError{
						Name:    n,
						Type:    t,
						Message: err.Error(),
					})
				}
			}()

			hf, err := FileToReader(f)
			if err != nil {
				return fmt.Errorf("failed to copyIn %v", err)
			}
			defer hf.Close()

			// ensure path exists
			if err := m.MkdirAll(filepath.Dir(n), 0777); err != nil {
				t = ErrCopyInCreateDir
				return fmt.Errorf("failed to create dir %v", err)
			}
			cf, err := m.Open(n, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0777)
			if err != nil {
				t = ErrCopyInCreateFile
				return err
			}
			defer cf.Close()

			_, err = io.Copy(cf, hf)
			if err != nil {
				t = ErrCopyInCopyContent
				return err
			}
			return nil
		})
	}
	return fileError, g.Wait()
}

func symlink(m Environment, symlinks map[string]string) (*FileError, error) {
	for k, v := range symlinks {
		if err := m.Symlink(v, k); err != nil {
			return &FileError{
				Name:    k,
				Type:    ErrSymlink,
				Message: err.Error(),
			}, err
		}
	}
	return nil, nil
}
