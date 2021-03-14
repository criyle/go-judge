package envexec

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/creack/pty"
)

// prepare Files for tty input / output
func prepareCmdFdTTY(c *Cmd, count int) (f []*os.File, p []pipeCollector, err error) {
	var wg sync.WaitGroup
	var hasInput, hasOutput bool

	fPty, fTty, err := pty.Open()
	if err != nil {
		err = fmt.Errorf("failed to open tty %v", err)
		return nil, nil, err
	}

	files := make([]*os.File, count)
	pipeToCollect := make([]pipeCollector, 0)

	defer func() {
		if err != nil {
			closeFiles(files...)
			closeFiles(fTty, fPty)
			wg.Wait()
		}
	}()

	for j, t := range c.Files {
		switch t := t.(type) {
		case nil: // ignore
		case *FileOpened:
			files[j] = t.File

		case *FileReader:
			if hasInput {
				return nil, nil, fmt.Errorf("cannot have multiple input when tty enabled")
			}
			hasInput = true

			files[j] = fTty

			// copy input
			wg.Add(1)
			go func() {
				defer wg.Done()
				io.Copy(fPty, t.Reader)
			}()

			// provide TTY
			if tty, ok := t.Reader.(ReaderTTY); ok {
				tty.TTY(fPty)
			}

		case *FileInput:
			var f *os.File
			f, err = os.Open(t.Path)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to open file %v", t.Path)
			}
			files[j] = f

		case *FilePipeCollector:
			files[j] = fTty
			if hasOutput {
				break
			}
			hasOutput = true

			done := make(chan struct{})
			buf := new(bytes.Buffer)
			pipeToCollect = append(pipeToCollect, pipeCollector{done, buf, t.Limit, t.Name})

			wg.Add(1)
			go func() {
				defer close(done)
				defer wg.Done()
				io.CopyN(buf, fPty, int64(t.Limit)+1)
			}()

		case *FileWriter:
			files[j] = fTty
			if hasOutput {
				break
			}
			hasOutput = true

			wg.Add(1)
			go func() {
				defer wg.Done()
				io.Copy(t.Writer, fPty)
			}()

		default:
			return nil, nil, fmt.Errorf("unknown file type %v %t", t, t)
		}
	}

	// ensure pty close after use
	go func() {
		wg.Wait()
		fPty.Close()
	}()
	return files, pipeToCollect, nil
}

func prepareCmdFd(c *Cmd, count int) (f []*os.File, p []pipeCollector, err error) {
	if c.TTY {
		return prepareCmdFdTTY(c, count)
	}
	files := make([]*os.File, count)
	pipeToCollect := make([]pipeCollector, 0)
	defer func() {
		if err != nil {
			closeFiles(files...)
		}
	}()
	// record same name buffer for one command to avoid multiple pipe creation
	pb := make(map[string]*pipeBuffer)

	for j, t := range c.Files {
		switch t := t.(type) {
		case nil: // ignore
		case *FileOpened:
			files[j] = t.File

		case *FileReader:
			if t.Stream {
				r, w, err := os.Pipe()
				if err != nil {
					return nil, nil, fmt.Errorf("failed to create pipe %v", err)
				}
				go w.ReadFrom(t.Reader)

				files[j] = r
			} else {
				f, err := readerToFile(t.Reader)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to open reader %v", err)
				}
				files[j] = f
			}

		case *FileInput:
			f, err := os.Open(t.Path)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to open file %v", t.Path)
			}
			files[j] = f

		case *FilePipeCollector:
			if b, ok := pb[t.Name]; ok {
				files[j] = b.W
				break
			}

			b, err := newPipeBuffer(t.Limit)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create pipe %v", err)
			}
			pb[t.Name] = b

			files[j] = b.W
			pipeToCollect = append(pipeToCollect, pipeCollector{b.Done, b.Buffer, t.Limit, t.Name})

		case *FileWriter:
			_, w, err := newPipe(t.Writer, t.Limit)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create pipe %v", err)
			}
			files[j] = w

		default:
			return nil, nil, fmt.Errorf("unknown file type %v %t", t, t)
		}
	}
	return files, pipeToCollect, nil
}

// prepareFd returns fds, pipeToCollect fileToClose, error
func prepareFds(r *Group) (f [][]*os.File, p [][]pipeCollector, err error) {
	// prepare fd count
	fdCount, err := countFd(r)
	if err != nil {
		return nil, nil, err
	}

	// prepare files
	files := make([][]*os.File, len(fdCount))
	pipeToCollect := make([][]pipeCollector, len(fdCount))

	// newly opened files need to be closed
	defer func() {
		if err != nil {
			for _, fs := range files {
				closeFiles(fs...)
			}
		}
	}()

	// prepare cmd fd
	for i, c := range r.Cmd {
		files[i], pipeToCollect[i], err = prepareCmdFd(c, fdCount[i])
		if err != nil {
			return nil, nil, err
		}
	}

	// prepare pipes
	for _, p := range r.Pipes {
		out, in, err := os.Pipe()
		if err != nil {
			return nil, nil, err
		}
		files[p.Out.Index][p.Out.Fd] = out
		files[p.In.Index][p.In.Fd] = in
	}
	return files, pipeToCollect, nil
}

func countFd(r *Group) ([]int, error) {
	fdCount := make([]int, len(r.Cmd))
	for i, c := range r.Cmd {
		fdCount[i] = len(c.Files)
	}
	for _, pi := range r.Pipes {
		for _, p := range []PipeIndex{pi.In, pi.Out} {
			if p.Index < 0 || p.Index >= len(r.Cmd) {
				return nil, fmt.Errorf("pipe index out of range %v", p.Index)
			}
			if p.Fd < len(r.Cmd[p.Index].Files) && r.Cmd[p.Index].Files[p.Fd] != nil {
				return nil, fmt.Errorf("pipe fd have been occupied %v %v", p.Index, p.Fd)
			}
			if p.Fd+1 > fdCount[p.Index] {
				fdCount[p.Index] = p.Fd + 1
			}
		}
	}
	return fdCount, nil
}
