package runner

import (
	"io/ioutil"
	"os"
	"sync"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/types"
)

func copyOutAndCollect(m container.Environment, c *Cmd, ptc []pipeBuff) (map[string]file.File, error) {
	// wait to complete
	var wg sync.WaitGroup
	wg.Add(len(ptc))

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
		wg.Add(len(cFiles))
	}

	// copy out
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
	for _, p := range ptc {
		go func(p pipeBuff) {
			defer wg.Done()
			<-p.buff.Done
			if int64(p.buff.Buffer.Len()) > p.buff.Max {
				writeErrorC(errC, types.StatusOutputLimitExceeded)
			}
			fc <- file.NewMemFile(p.name, p.buff.Buffer.Bytes())
		}(p)
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
