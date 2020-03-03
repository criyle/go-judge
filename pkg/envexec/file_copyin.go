package envexec

import (
	"io"
	"os"
	"sync"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-sandbox/container"
)

func copyIn(m container.Environment, copyIn map[string]file.File) error {
	// open copyin files
	openCmd := make([]container.OpenCmd, 0, len(copyIn))
	files := make([]file.File, 0, len(copyIn))
	for n, f := range copyIn {
		openCmd = append(openCmd, container.OpenCmd{
			Path: n,
			Flag: os.O_CREATE | os.O_RDWR | os.O_TRUNC,
			Perm: 0777,
		})
		files = append(files, f)
	}

	// open files from container
	cFiles, err := m.Open(openCmd)
	if err != nil {
		return err
	}

	// copyin in parallel
	var wg sync.WaitGroup
	errC := make(chan error, 1)
	wg.Add(len(files))
	for i, f := range files {
		go func(cFile *os.File, hFile file.File) {
			defer wg.Done()
			defer cFile.Close()

			// open host file
			hf, err := hFile.Reader()
			if err != nil {
				writeErrorC(errC, err)
				return
			}
			defer hf.Close()

			// copy to container
			_, err = io.Copy(cFile, hf)
			if err != nil {
				writeErrorC(errC, err)
				return
			}
		}(cFiles[i], f)
	}
	wg.Wait()

	// check error
	select {
	case err := <-errC:
		return err
	default:
	}
	return nil
}
