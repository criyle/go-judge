package envexec

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/creack/pty"
)

var closedChan chan struct{}

func init() {
	closedChan = make(chan struct{})
	close(closedChan)
}

// prepare Files for tty input / output
func prepareCmdFdTTY(c *Cmd, count int, newStoreFile NewStoreFile) (f []*os.File, p []pipeCollector, err error) {
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

		case *FileCollector:
			files[j] = fTty
			if hasOutput {
				break
			}
			hasOutput = true

			done := make(chan struct{})
			buf, err := newStoreFile()
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create store file %v", err)
			}
			pipeToCollect = append(pipeToCollect, pipeCollector{done, buf, t.Limit, t.Name, true})

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

func prepareCmdFd(c *Cmd, count int, newFileStore NewStoreFile) (f []*os.File, p []pipeCollector, err error) {
	if c.TTY {
		return prepareCmdFdTTY(c, count, newFileStore)
	}
	files := make([]*os.File, count)
	pipeToCollect := make([]pipeCollector, 0)
	defer func() {
		if err != nil {
			closeFiles(files...)
			for _, p := range pipeToCollect {
				<-p.done
				p.buffer.Close()
			}
		}
	}()
	// record the same file to avoid multiple file open
	cf := make(map[string]*os.File)

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

		case *FileCollector:
			if f, ok := cf[t.Name]; ok {
				files[j] = f
				break
			}

			if t.Pipe {
				b, err := newPipeBuffer(t.Limit, newFileStore)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to create pipe %v", err)
				}
				cf[t.Name] = b.W

				files[j] = b.W
				pipeToCollect = append(pipeToCollect, pipeCollector{b.Done, b.Buffer, t.Limit, t.Name, true})
			} else {
				f, err := c.Environment.Open(t.Name, os.O_CREATE|os.O_WRONLY, 0777)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to create container file %v", err)
				}
				cf[t.Name] = f

				buffer, err := c.Environment.Open(t.Name, os.O_RDWR, 0777)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to open created container file %v", err)
				}

				files[j] = f
				pipeToCollect = append(pipeToCollect, pipeCollector{closedChan, buffer, t.Limit, t.Name, false})
			}

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
func prepareFds(r *Group, newStoreFile NewStoreFile) (f [][]*os.File, p [][]pipeCollector, err error) {
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
		files[i], pipeToCollect[i], err = prepareCmdFd(c, fdCount[i], newStoreFile)
		if err != nil {
			return nil, nil, err
		}
	}

	// prepare pipes
	for _, p := range r.Pipes {
		out, in, pc, err := pipe(p, newStoreFile)
		if err != nil {
			return nil, nil, err
		}
		files[p.Out.Index][p.Out.Fd] = out
		files[p.In.Index][p.In.Fd] = in

		if pc != nil {
			pipeToCollect[p.In.Index] = append(pipeToCollect[p.In.Index], *pc)
		}
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

func pipe(p Pipe, newStoreFile NewStoreFile) (out *os.File, in *os.File, pc *pipeCollector, err error) {
	if p.Proxy {
		buffer, err := newStoreFile()
		if err != nil {
			return nil, nil, nil, err
		}
		out1, in1, out2, in2, err := pipe2()
		if err != nil {
			buffer.Close()
			return nil, nil, nil, err
		}

		pc := pipeProxy(p, out1, in2, buffer)
		return out2, in1, pc, nil
	}

	out, in, err = os.Pipe()
	if err != nil {
		return nil, nil, nil, err
	}
	return out, in, nil, nil
}

func pipe2() (out1 *os.File, in1 *os.File, out2 *os.File, in2 *os.File, err error) {
	out1, in1, err = os.Pipe()
	if err != nil {
		return
	}
	out2, in2, err = os.Pipe()
	if err != nil {
		out1.Close()
		in1.Close()
		return
	}
	return
}

func pipeProxy(p Pipe, out1 *os.File, in2 *os.File, buffer *os.File) *pipeCollector {
	copyAndClose := func() {
		io.Copy(in2, out1)
		in2.Close()
		io.Copy(io.Discard, out1)
		out1.Close()
	}

	// if no name, simply copy data
	if p.Name == "" {
		go copyAndClose()
		return nil
	}

	done := make(chan struct{})
	limit := p.Limit

	// out1 -> in2
	go func() {
		// copy with limit
		r := io.TeeReader(io.LimitReader(out1, int64(limit)), buffer)
		io.Copy(in2, r)
		close(done)

		// copy without limit
		copyAndClose()
	}()

	return &pipeCollector{
		done:    done,
		buffer:  buffer,
		limit:   p.Limit,
		name:    p.Name,
		storage: true,
	}
}
