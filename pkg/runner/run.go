package runner

import (
	"fmt"
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

// fds will be closed
func runOne(m *daemon.Master, cg *cgroup.CGroup, c *Cmd, fds []*os.File, ptc []pipeBuff) (<-chan Result, error) {
	fdToClose := fds
	defer func() {
		closeFiles(fdToClose)
	}()

	// setup cgroup limits
	cg.SetMemoryLimitInBytes(c.MemoryLimit + memoryLimitExtra)
	cg.SetPidsMax(c.PidLimit)

	// copyin files
	for n, f := range c.CopyIn {
		fi, err := f.Open()
		if err != nil {
			return nil, err
		}
		m.CopyIn(fi, n)
		fi.Close()
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
	var tle bool

	// wait to finish
	// 1. cmd exit first, signal waiter to exit
	// 2. waiter exit first, signal proc to exit
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
	rt := make(map[string]file.File)
	fc := make(chan file.File)
	errC := make(chan error, 1) // collect only 1 error
	collected := make(chan struct{})
	// collect to map
	go func() {
		for f := range fc {
			name := f.Name()
			rt[name] = f
		}
		close(collected)
	}()
	// wait to complete
	var wg sync.WaitGroup
	wg.Add(len(c.CopyOut) + len(ptc))

	putErr := func(err error) {
		select {
		case errC <- err:
		default:
		}
	}

	// copy out
	for _, n := range c.CopyOut {
		go func(n string) {
			defer wg.Done()
			f, err := m.Open(n)
			if err != nil {
				putErr(err)
				return
			}
			defer f.Close()
			c, err := ioutil.ReadAll(f)
			if err != nil {
				putErr(err)
				return
			}
			fc <- memfile.New(n, c)
		}(n)
	}

	// collect pipe
	for _, p := range ptc {
		go func(p pipeBuff) {
			defer wg.Done()
			<-p.buff.Done
			if int64(p.buff.Buffer.Len()) > p.buff.Max {
				putErr(types.StatusOLE)
			}
			fc <- memfile.New(p.name, p.buff.Buffer.Bytes())
		}(p)
	}

	wg.Wait()
	close(fc)

	// check error
	var err error
	select {
	case err = <-errC:
	default:
	}
	// wait collected
	<-collected
	return rt, err
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
