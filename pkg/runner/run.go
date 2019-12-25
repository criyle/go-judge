package runner

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-judge/file/memfile"
	"github.com/criyle/go-sandbox/daemon"
	"github.com/criyle/go-sandbox/pkg/cgroup"
	"github.com/criyle/go-sandbox/pkg/pipe"
	"github.com/criyle/go-sandbox/types"
)

var memoryLimitExtra uint64 = 16 << 10 // 16k more memory

type pipeBuff struct {
	buff *pipe.Buffer
	name string
}

// Run starts the cmd and returns results
func (r *Runner) Run() ([]Result, error) {
	var (
		fileToClose []*os.File
	)

	defer func() {
		closeFiles(fileToClose)
	}()

	// prepare fd count
	fdCount, err := countFd(r)
	if err != nil {
		return nil, err
	}

	// prepare files
	fds := make([][]*os.File, len(fdCount))
	pipeToCollect := make([][]pipeBuff, len(fdCount))
	for i, c := range r.Cmds {
		fds[i] = make([]*os.File, fdCount[i])
		for j, t := range c.Files {
			if t == nil {
				continue
			}
			switch t := t.(type) {
			case *os.File:
				fds[i][j] = t
				fileToClose = append(fileToClose, t)

			case file.Opener:
				f, err := t.Open()
				if err != nil {
					return nil, fmt.Errorf("fail to open file %v", t)
				}
				fds[i][j] = f
				fileToClose = append(fileToClose, f)

			case PipeCollector:
				b, err := pipe.NewBuffer(t.SizeLimit)
				if err != nil {
					return nil, fmt.Errorf("failed to create pipe %v", err)
				}
				fds[i][j] = b.W
				pipeToCollect[i] = append(pipeToCollect[i], pipeBuff{b, t.Name})
				fileToClose = append(fileToClose, b.W)

			default:
				return nil, fmt.Errorf("unknown file type %v", t)
			}
		}
	}

	// prepare pipes
	for _, p := range r.Pipes {
		out, in, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		fileToClose = append(fileToClose, out, in)
		fds[p.Out.Index][p.Out.Fd] = out
		fds[p.In.Index][p.In.Fd] = in
	}

	// prepare masters
	ms := make([]*daemon.Master, 0, len(r.Cmds))
	for range r.Cmds {
		m, err := r.MasterPool.Get()
		if err != nil {
			return nil, fmt.Errorf("failed to get master %v", err)
		}
		defer r.MasterPool.Put(m)
		ms = append(ms, m)
	}

	// prepare cgroup
	cgs := make([]*cgroup.CGroup, 0, len(r.Cmds))
	for range r.Cmds {
		cg, err := r.CGBuilder.Build()
		if err != nil {
			return nil, fmt.Errorf("failed to build cgroup %v", err)
		}
		defer cg.Destroy()
		cgs = append(cgs, cg)
	}

	// run cmds
	errC := make(chan error, 1)
	var wg sync.WaitGroup
	result := make([]Result, len(r.Cmds))
	wg.Add(len(r.Cmds))
	for i, c := range r.Cmds {
		go func(i int, c *Cmd) {
			defer wg.Done()
			r, err := runOne(ms[i], cgs[i], c, fds[i], pipeToCollect[i])
			if err != nil {
				select {
				case errC <- err:
				default:
				}
				return
			}
			result[i] = <-r
		}(i, c)
	}
	wg.Wait()
	fileToClose = nil // already closed by runOne

	// collect potential error
	select {
	case err = <-errC:
	default:
	}
	return result, err
}

func writeErrorC(errC chan error, err error) {
	select {
	case errC <- err:
	default:
	}
}

// fds will be closed
func runOne(m *daemon.Master, cg *cgroup.CGroup, c *Cmd, fds []*os.File, ptc []pipeBuff) (<-chan Result, error) {
	fdToClose := fds
	defer func() {
		closeFiles(fdToClose)
	}()

	// setup cgroup limits
	cg.SetMemoryLimitInBytes(c.MemoryLimit + memoryLimitExtra)
	cg.SetPidsMax(c.PidLimit)

	// copyin
	if len(c.CopyIn) > 0 {
		// open copyin files
		openCmd := make([]daemon.OpenCmd, 0, len(c.CopyIn))
		files := make([]file.File, 0, len(c.CopyIn))
		for n, f := range c.CopyIn {
			openCmd = append(openCmd, daemon.OpenCmd{
				Path: n,
				Flag: os.O_CREATE | os.O_RDWR | os.O_TRUNC,
				Perm: 0777,
			})
			files = append(files, f)
		}

		// open files from container
		cFiles, err := m.Open(openCmd)
		if err != nil {
			return nil, err
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
				hf, err := hFile.Open()
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
			return nil, err
		default:
		}
	}

	// set running parameters
	execParam := &daemon.ExecveParam{
		Args:     c.Args,
		Env:      c.Env,
		Fds:      getFdArray(fds),
		SyncFunc: cg.AddProc,
	}

	// start the cmd
	done := make(chan struct{})
	finish := make(chan struct{})
	rc, err := m.Execve(done, execParam)
	if err != nil {
		return nil, err
	}

	// close files
	closeFiles(fds)
	fdToClose = nil
	result := make(chan Result, 1)

	// wait to finish
	// 1. cmd exit first, signal waiter to exit
	// 2. waiter exit first, signal proc to exit
	var tle bool
	go func() {
		tle = c.Waiter(finish, cg)
		close(done)
	}()

	go func() {
		rt := <-rc
		close(finish)
		// collect result
		files, err := copyOutAndCollect(m, c, ptc)
		re := Result{
			Status: rt.Status,
			Error:  rt.Error,
			Time:   time.Duration(rt.UserTime),
			Memory: rt.UserMem << 10,
			Files:  files,
		}
		// collect error
		if err != nil && re.Error == "" {
			switch err := err.(type) {
			case types.Status:
				re.Status = err
			default:
				re.Status = types.StatusFatal
			}
			re.Error = err.Error()
		}
		// time
		cpuUsage, err := cg.CpuacctUsage()
		if err != nil {
			re.Status = types.StatusFatal
			re.Error = err.Error()
		} else {
			re.Time = time.Duration(cpuUsage)
		}
		// memory
		memoryUsage, err := cg.MemoryMaxUsageInBytes()
		if err != nil {
			re.Status = types.StatusFatal
			re.Error = err.Error()
		} else {
			re.Memory = memoryUsage
		}
		// wait waiter done
		<-done
		if tle {
			re.Status = types.StatusTLE
		}
		if re.Memory > c.MemoryLimit {
			re.Status = types.StatusMLE
		}
		result <- re
	}()

	return result, nil
}

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

func getFdArray(fd []*os.File) []uintptr {
	r := make([]uintptr, 0, len(fd))
	for _, f := range fd {
		r = append(r, f.Fd())
	}
	return r
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

func closeFiles(files []*os.File) {
	for _, f := range files {
		f.Close()
	}
}
