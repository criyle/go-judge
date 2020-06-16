package envexec

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/creack/pty"
	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-sandbox/pkg/pipe"
)

type pipeCollector struct {
	buff *pipe.Buffer
	name string
}

// prepare Files for tty input / output
func prepareCmdFdTTY(c *Cmd, count int) (fd, ftc []*os.File, ptc []pipeCollector, err error) {
	var fPty, fTty *os.File
	if c.TTY {
		fPty, fTty, err = pty.Open()
		if err != nil {
			err = fmt.Errorf("failed to open tty %v", err)
			return nil, nil, nil, err
		}
	}
	ftc = append(ftc, fTty)

	fd = make([]*os.File, count)
	hasInput := false
	var output *pipe.Buffer
	var wg sync.WaitGroup

	for j, t := range c.Files {
		switch t := t.(type) {
		case nil: // ignore
		case *os.File:
			fd[j] = t
			ftc = append(ftc, t)

		case file.ReaderOpener:
			if hasInput {
				err = fmt.Errorf("cannot have multiple input when tty enabled")
				goto openError
			}
			hasInput = true

			r, err := t.Reader()
			if err != nil {
				err = fmt.Errorf("failed to open file %v", t)
				goto openError
			}
			fd[j] = fTty

			// copy input
			wg.Add(1)
			go func() {
				defer wg.Done()
				io.Copy(fPty, r)
			}()

		case PipeCollector:
			fd[j] = fTty
			if output != nil {
				break
			}

			done := make(chan struct{})
			output = &pipe.Buffer{
				W:      fTty,
				Max:    t.SizeLimit,
				Buffer: new(bytes.Buffer),
				Done:   done,
			}
			ptc = append(ptc, pipeCollector{output, t.Name})

			wg.Add(1)
			go func() {
				defer close(done)
				defer wg.Done()
				io.CopyN(output.Buffer, fPty, output.Max+1)
			}()

		default:
			err = fmt.Errorf("unknown file type %v %t", t, t)
			goto openError
		}
	}

	// ensure pty close after use
	go func() {
		wg.Wait()
		fPty.Close()
	}()

	return

openError:
	closeFiles(ftc)
	return nil, nil, nil, err
}

func prepareCmdFd(c *Cmd, count int) (fd, ftc []*os.File, ptc []pipeCollector, err error) {
	if c.TTY {
		return prepareCmdFdTTY(c, count)
	}
	fd = make([]*os.File, count)
	// record same name buffer for one command to avoid multiple pipe creation
	pb := make(map[string]*pipe.Buffer)

	for j, t := range c.Files {
		switch t := t.(type) {
		case nil: // ignore
		case *os.File:
			fd[j] = t
			ftc = append(ftc, t)

		case file.Opener:
			f, err := t.Open()
			if err != nil {
				err = fmt.Errorf("failed to open file %v", t)
				goto openError
			}
			fd[j] = f
			ftc = append(ftc, f)

		case PipeCollector:
			if b, ok := pb[t.Name]; ok {
				fd[j] = b.W
				break
			}

			b, err := pipe.NewBuffer(t.SizeLimit)
			if err != nil {
				err = fmt.Errorf("failed to create pipe %v", err)
				goto openError
			}
			fd[j] = b.W
			pb[t.Name] = b
			ptc = append(ptc, pipeCollector{b, t.Name})
			ftc = append(ftc, b.W)

		default:
			err = fmt.Errorf("unknown file type %v %t", t, t)
			goto openError
		}
	}
	return

openError:
	closeFiles(ftc)
	return nil, nil, nil, err
}

// prepareFd returns fds, pipeToCollect fileToClose, error
func prepareFds(r *Group) ([][]*os.File, [][]pipeCollector, []*os.File, error) {
	// prepare fd count
	fdCount, err := countFd(r)
	if err != nil {
		return nil, nil, nil, err
	}

	// newly opened files need to be closed
	var fileToClose []*os.File

	// prepare files
	fds := make([][]*os.File, len(fdCount))
	pipeToCollect := make([][]pipeCollector, len(fdCount))
	// prepare cmd fd
	for i, c := range r.Cmd {
		var ftc []*os.File
		fds[i], ftc, pipeToCollect[i], err = prepareCmdFd(c, fdCount[i])
		if err != nil {
			return nil, nil, nil, err
		}
		fileToClose = append(fileToClose, ftc...)
	}

	// prepare pipes
	for _, p := range r.Pipes {
		out, in, err := os.Pipe()
		if err != nil {
			return nil, nil, nil, err
		}
		fileToClose = append(fileToClose, out, in)
		fds[p.Out.Index][p.Out.Fd] = out
		fds[p.In.Index][p.In.Fd] = in
	}
	return fds, pipeToCollect, fileToClose, nil
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
