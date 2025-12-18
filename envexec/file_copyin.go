package envexec

import (
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/sync/errgroup"
)

// copyIn copied file from host to container in parallel
func copyIn(m Environment, copyIn map[string]File) ([]FileError, error) {
	var (
		g          errgroup.Group
		fileErrors []FileError
		mu         sync.Mutex
	)

	names := make([]string, 0, len(copyIn))
	cmds := make([]OpenParam, 0, len(copyIn))
	for n := range copyIn {
		names = append(names, n)
		cmds = append(cmds, OpenParam{
			Path:     n,
			Flag:     os.O_CREATE | os.O_WRONLY | os.O_TRUNC,
			Perm:     0777,
			MkdirAll: true,
		})
	}

	results, err := m.Open(cmds)
	if err != nil {
		return nil, fmt.Errorf("copyin: batch open failed: %w", err)
	}

	for i, res := range results {
		i, res := i, res
		fileName := names[i]
		sourceFile := copyIn[fileName]

		// Handle specific open/mkdir errors from the container
		if res.Err != nil {
			mu.Lock()
			fileErrors = append(fileErrors, FileError{
				Name:    fileName,
				Type:    ErrCopyInCreateFile,
				Message: res.Err.Error(),
			})
			mu.Unlock()
			continue
		}

		// If open was successful, start the copy in a goroutine
		g.Go(func() error {
			remoteFile := res.File
			defer remoteFile.Close()

			hf, err := FileToReader(sourceFile)
			if err != nil {
				mu.Lock()
				fileErrors = append(fileErrors, FileError{
					Name: fileName, Type: ErrCopyInOpenFile, Message: err.Error(),
				})
				mu.Unlock()
				return nil // Don't return error to g.Wait to allow other copies to finish
			}
			defer hf.Close()

			if _, err := io.Copy(remoteFile, hf); err != nil {
				mu.Lock()
				fileErrors = append(fileErrors, FileError{
					Name: fileName, Type: ErrCopyInCopyContent, Message: err.Error(),
				})
				mu.Unlock()
			}
			return nil
		})
	}
	return fileErrors, g.Wait()
}

func symlink(m Environment, symlinks map[string]string) ([]FileError, error) {
	if len(symlinks) == 0 {
		return nil, nil
	}

	batch := make([]SymlinkParam, 0, len(symlinks))
	for k, v := range symlinks {
		batch = append(batch, SymlinkParam{
			LinkPath: k,
			Target:   v,
		})
	}

	errs := m.Symlink(batch)
	var fileErrors []FileError
	for i, err := range errs {
		if err != nil {
			fileErrors = append(fileErrors, FileError{
				Name:    batch[i].LinkPath,
				Type:    ErrSymlink,
				Message: fmt.Sprintf("symlink: %q -> %q: %s", batch[i].LinkPath, batch[i].Target, err.Error()),
			})
		}
	}

	if len(fileErrors) > 0 {
		return fileErrors, fmt.Errorf("symlink: %d operations failed", len(fileErrors))
	}
	return nil, nil
}
