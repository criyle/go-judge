package envexec

import (
	"fmt"
	"os"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-sandbox/pkg/pipe"
)

type pipeCollector struct {
	buff *pipe.Buffer
	name string
}

func prepareCmdFd(c *Cmd, count int) (fd, ftc []*os.File, ptc []pipeCollector, err error) {
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
				err = fmt.Errorf("fail to open file %v", t)
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
