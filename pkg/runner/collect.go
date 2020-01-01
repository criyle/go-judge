package runner

import (
	"io/ioutil"
	"os"
	"sync"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-judge/file/memfile"
	"github.com/criyle/go-sandbox/daemon"
	"github.com/criyle/go-sandbox/types"
)

func copyOutAndCollect(m *daemon.Master, c *Cmd, ptc []pipeBuff) (map[string]file.File, error) {
	total := len(c.CopyOut) + len(ptc)

	fc := make(chan file.File, total)
	errC := make(chan error, 1) // collect only 1 error

	var (
		cFiles []*os.File
		err    error
	)
	if len(c.CopyOut) > 0 {
		// prepare open param
		openCmd := make([]daemon.OpenCmd, 0, len(c.CopyOut))
		for _, n := range c.CopyOut {
			openCmd = append(openCmd, daemon.OpenCmd{
				Path: n,
				Flag: os.O_RDONLY,
			})
		}

		// open all
		cFiles, err = m.Open(openCmd)
		if err != nil {
			return nil, err
		}
	}

	// wait to complete
	var wg sync.WaitGroup
	wg.Add(total)

	// copy out
	for i, n := range c.CopyOut {
		go func(cFile *os.File, n string) {
			defer wg.Done()
			defer cFile.Close()
			c, err := ioutil.ReadAll(cFile)
			if err != nil {
				writeErrorC(errC, err)
				return
			}
			fc <- memfile.New(n, c)
		}(cFiles[i], n)
	}

	// collect pipe
	for _, p := range ptc {
		go func(p pipeBuff) {
			defer wg.Done()
			<-p.buff.Done
			if int64(p.buff.Buffer.Len()) > p.buff.Max {
				writeErrorC(errC, types.StatusOLE)
			}
			fc <- memfile.New(p.name, p.buff.Buffer.Bytes())
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

	// check error
	select {
	case err := <-errC:
		return rt, err
	default:
	}

	return rt, nil
}
