package runner

import (
	"fmt"
	"os"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-sandbox/pkg/pipe"
)

// prepareFd returns fds, pipeToCollect fileToClose, error
func prepareFds(r *Runner) ([][]*os.File, [][]pipeBuff, []*os.File, error) {
	// prepare fd count
	fdCount, err := countFd(r)
	if err != nil {
		return nil, nil, nil, err
	}

	// newly opened files need to be closed
	var fileToClose []*os.File

	// prepare files
	fds := make([][]*os.File, len(fdCount))
	pipeToCollect := make([][]pipeBuff, len(fdCount))

	prepareCmdFd := func(c *Cmd, count int) ([]*os.File, []pipeBuff, error) {
		fd := make([]*os.File, count)
		var ptc []pipeBuff

		for j, t := range c.Files {
			if t == nil {
				continue
			}
			switch t := t.(type) {
			case *os.File:
				fd[j] = t
				fileToClose = append(fileToClose, t)

			case file.Opener:
				f, err := t.Open()
				if err != nil {
					return nil, nil, fmt.Errorf("fail to open file %v", t)
				}
				fd[j] = f
				fileToClose = append(fileToClose, f)

			case PipeCollector:
				b, err := pipe.NewBuffer(t.SizeLimit)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to create pipe %v", err)
				}
				fd[j] = b.W
				ptc = append(ptc, pipeBuff{b, t.Name})
				fileToClose = append(fileToClose, b.W)

			default:
				return nil, nil, fmt.Errorf("unknown file type %v", t)
			}
		}
		return fd, ptc, nil
	}

	// prepare cmd fd
	for i, c := range r.Cmds {
		fds[i], pipeToCollect[i], err = prepareCmdFd(c, fdCount[i])
		if err != nil {
			return nil, nil, nil, err
		}
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

func countFd(r *Runner) ([]int, error) {
	fdCount := make([]int, len(r.Cmds))
	for i, c := range r.Cmds {
		fdCount[i] = len(c.Files)
	}
	for _, pi := range r.Pipes {
		for _, p := range []PipeIndex{pi.In, pi.Out} {
			if p.Index < 0 || p.Index >= len(r.Cmds) {
				return nil, fmt.Errorf("pipe index out of range %v", p.Index)
			}
			if p.Fd < len(r.Cmds[p.Index].Files) && r.Cmds[p.Index].Files[p.Fd] != nil {
				return nil, fmt.Errorf("pipe fd have been occupied %v %v", p.Index, p.Fd)
			}
			if p.Fd+1 > fdCount[p.Index] {
				fdCount[p.Index] = p.Fd + 1
			}
		}
	}
	return fdCount, nil
}
