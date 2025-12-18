package envexec

import (
	"fmt"
	"io"
	"os"

	"github.com/creack/pty"
)

var closedChan chan struct{}

func init() {
	closedChan = make(chan struct{})
	close(closedChan)
}

func prepareCmdFd(c *Cmd, count int, newFileStore NewStoreFile) (f []*os.File, p []pipeCollector, err error) {
	if c.TTY {
		return prepareCmdFdTTY(c, count, newFileStore)
	}
	return prepareCmdFdNoTty(c, count, newFileStore)
}

// prepare Files for tty input / output
func prepareCmdFdTTY(c *Cmd, count int, newStoreFile NewStoreFile) (f []*os.File, p []pipeCollector, err error) {
	var hasInput, hasOutput bool

	fPty, fTty, err := pty.Open()
	if err != nil {
		err = fmt.Errorf("failed to open tty %v", err)
		return nil, nil, err
	}
	sf := newSharedFile(fPty)

	files := make([]*os.File, count)
	pipeToCollect := make([]pipeCollector, 0)

	defer func() {
		if err != nil {
			closeFiles(files...)
			closeFiles(fTty, fPty)
		}
	}()

	for j, t := range c.Files {
		switch t := t.(type) {
		case nil: // ignore
		case *FileOpened:
			files[j] = t.File

		case *fileStreamIn:
			if hasInput {
				return nil, nil, fmt.Errorf("tty: multiple input not allowed")
			}
			hasInput = true

			files[j] = fTty
			// provide TTY
			sf.Acquire()
			t.start(sf)

		case *fileStreamOut:
			files[j] = fTty
			hasOutput = true
			// Provide TTY
			sf.Acquire()
			t.start(sf)

		case *FileReader:
			if hasInput {
				return nil, nil, fmt.Errorf("tty: multiple input not allowed")
			}
			hasInput = true

			files[j] = fTty

			// copy input
			sf.Acquire()
			go func() {
				defer sf.Close()
				io.Copy(fPty, t.Reader)
			}()

		case *FileInput:
			var f *os.File
			f, err = os.Open(t.Path)
			if err != nil {
				return nil, nil, fmt.Errorf("tty: open file %v: %w", t.Path, err)
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
				return nil, nil, fmt.Errorf("tty: create store file: %w", err)
			}
			pipeToCollect = append(pipeToCollect, pipeCollector{done, buf, t.Limit, t.Name, true})

			sf.Acquire()
			go func() {
				defer close(done)
				defer sf.Close()
				io.CopyN(buf, fPty, int64(t.Limit)+1)
			}()

		case *FileWriter:
			files[j] = fTty
			if hasOutput {
				break
			}
			hasOutput = true

			sf.Acquire()
			go func() {
				defer sf.Close()
				io.Copy(t.Writer, fPty)
			}()

		default:
			return nil, nil, fmt.Errorf("tty: unknown file type: %T", t)
		}
	}
	return files, pipeToCollect, nil
}

func prepareCmdFdNoTty(c *Cmd, count int, newFileStore NewStoreFile) (f []*os.File, p []pipeCollector, err error) {
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
	prepareFileCollector := func(t *FileCollector) (*os.File, error) {
		if f, ok := cf[t.Name]; ok {
			return f, nil
		}

		if t.Pipe {
			b, err := newPipeBuffer(t.Limit, newFileStore)
			if err != nil {
				return nil, fmt.Errorf("pipe: create: %w", err)
			}
			cf[t.Name] = b.W
			pipeToCollect = append(pipeToCollect, pipeCollector{b.Done, b.Buffer, t.Limit, t.Name, true})
			return b.W, nil
		} else {
			f, err := c.Environment.Open(t.Name, os.O_CREATE|os.O_WRONLY, 0777)
			if err != nil {
				return nil, fmt.Errorf("container: create file %v: %w", t.Name, err)
			}
			buffer, err := c.Environment.Open(t.Name, os.O_RDWR, 0777)
			if err != nil {
				f.Close()
				return nil, fmt.Errorf("container: open created file %v: %w", t.Name, err)
			}
			cf[t.Name] = f
			pipeToCollect = append(pipeToCollect, pipeCollector{closedChan, buffer, t.Limit, t.Name, false})
			return f, nil
		}
	}

	for j, t := range c.Files {
		switch t := t.(type) {
		case nil: // ignore
		case *FileOpened:
			files[j] = t.File

		case *FileReader:
			f, err := readerToFile(t.Reader)
			if err != nil {
				return nil, nil, fmt.Errorf("pipe: open reader: %w", err)
			}
			files[j] = f

		case *FileInput:
			f, err := os.Open(t.Path)
			if err != nil {
				return nil, nil, fmt.Errorf("pipe: open file %v: %w", t.Path, err)
			}
			files[j] = f

		case *FileCollector:
			f, err := prepareFileCollector(t)
			if err != nil {
				return nil, nil, err
			}
			files[j] = f

		case *FileWriter:
			_, w, err := newPipe(t.Writer, t.Limit)
			if err != nil {
				return nil, nil, fmt.Errorf("pipe: create: %w", err)
			}
			files[j] = w

		case *fileStreamIn:
			out, in, err := os.Pipe()
			if err != nil {
				return nil, nil, fmt.Errorf("pipe: stream in create: %w", err)
			}
			files[j] = out
			sf := newSharedFile(in)
			sf.Acquire()
			t.start(sf)

		case *fileStreamOut:
			out, in, err := os.Pipe()
			if err != nil {
				return nil, nil, fmt.Errorf("pipe: stream out create: %w", err)
			}
			files[j] = in
			sf := newSharedFile(out)
			sf.Acquire()
			t.start(sf)

		default:
			return nil, nil, fmt.Errorf("unknown file type: %T", t)
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
			return nil, nil, fmt.Errorf("prepare fds: prepareCmdFd: %w", err)
		}
	}

	// prepare pipes
	for _, p := range r.Pipes {
		if files[p.Out.Index][p.Out.Fd] != nil {
			return nil, nil, fmt.Errorf("pipe: mapping to existing file descriptor: out %d/%d", p.Out.Index, p.Out.Fd)
		}
		if files[p.In.Index][p.In.Fd] != nil {
			return nil, nil, fmt.Errorf("pipe: mapping to existing file descriptor: in %d/%d", p.In.Index, p.In.Fd)
		}
		out, in, pc, err := pipe(p, newStoreFile)
		if err != nil {
			return nil, nil, fmt.Errorf("pipe: create: %w", err)
		}
		files[p.Out.Index][p.Out.Fd] = out
		files[p.In.Index][p.In.Fd] = in

		if pc != nil {
			pipeToCollect[p.In.Index] = append(pipeToCollect[p.In.Index], *pc)
		}
	}

	// null check
	for i, fds := range files {
		for j, f := range fds {
			if f == nil {
				return nil, nil, fmt.Errorf("null file for index/fd: %d/%d", i, j)
			}
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
				return nil, fmt.Errorf("pipe: index out of range %v", p.Index)
			}
			if p.Fd < len(r.Cmd[p.Index].Files) && r.Cmd[p.Index].Files[p.Fd] != nil {
				return nil, fmt.Errorf("pipe: fd already occupied %v %v", p.Index, p.Fd)
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
			return nil, nil, nil, fmt.Errorf("pipe: create store file: %w", err)
		}
		out1, in1, out2, in2, err := pipe2()
		if err != nil {
			buffer.Close()
			return nil, nil, nil, fmt.Errorf("pipe: create: %w", err)
		}
		if p.DisableZeroCopy {
			pc = pipeProxy(p, out1, in2, buffer)
		} else {
			pc = pipeProxyZeroCopy(p, out1, in2, buffer)
		}
		return out2, in1, pc, nil
	}

	out, in, err = os.Pipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("pipe: create: %w", err)
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
		lr := io.LimitReader(out1, int64(limit))
		r := io.TeeReader(lr, buffer)

		n, _ := io.Copy(in2, r)
		if n < int64(limit) {
			io.Copy(buffer, lr)
		}
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
