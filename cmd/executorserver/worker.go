package main

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-judge/pkg/envexec"
	"github.com/criyle/go-sandbox/runner"
)

const maxWaiting = 512

type workRequest struct {
	*request
	resultCh chan<- []response
}

var (
	workStartOnce sync.Once
	workStopOnce  sync.Once

	workCh chan workRequest
	workWg sync.WaitGroup

	workCtx       context.Context
	workCtxCancel func()
)

func startWorkers() {
	workStartOnce.Do(func() {
		workCtx, workCtxCancel = context.WithCancel(context.Background())
		workCh = make(chan workRequest, maxWaiting)
		workWg.Add(*parallism)
		for i := 0; i < *parallism; i++ {
			go workerLoop()
		}
	})
}

func workerLoop() {
	defer workWg.Done()
	for {
		var req workRequest
		select {
		case <-workCtx.Done():
			return
		case req = <-workCh:
		}
		workDoCmd(req)
	}
}

func workDoCmd(req workRequest) {
	if len(req.Cmd) == 1 {
		req.resultCh <- []response{workDoSingle(req.Cmd[0])}
	} else {
		req.resultCh <- workDoGroup(req.Cmd, req.PipeMapping)
	}
}

func workDoSingle(rc cmd) (res response) {
	c, copyOutSet, err := prepareCmd(rc)
	if err != nil {
		res.Status = status(envexec.StatusInternalError)
		res.Error = err.Error()
		return
	}
	s := &envexec.Single{
		CgroupPool:      cgroupPool,
		EnvironmentPool: envPool,
		Cmd:             c,
	}
	result, err := s.Run()
	if err != nil {
		res.Status = status(envexec.StatusInternalError)
		res.Error = err.Error()
		return
	}
	res.Status = status(result.Status)
	res.ExitStatus = result.ExitStatus
	res.Error = result.Error
	res.Time = uint64(result.Time)
	res.Memory = uint64(result.Memory)
	res.Files = make(map[string]string)
	res.FileIDs = make(map[string]string)

	for name, fi := range result.Files {
		b, err := fi.Content()
		if err != nil {
			res.Status = status(envexec.StatusFileError)
			res.Error = err.Error()
			return
		}
		if copyOutSet[name] {
			res.Files[name] = string(b)
		} else {
			id, err := fs.Add(name, b)
			if err != nil {
				res.Status = status(envexec.StatusFileError)
				res.Error = err.Error()
				return
			}
			res.FileIDs[name] = id
		}
	}
	return
}

func workDoGroup(rc []cmd, pm []pipeMap) (rts []response) {
	p := preparePipeMapping(pm)
	cs := make([]*envexec.Cmd, 0, len(rc))
	copyOutSets := make([]map[string]bool, 0, len(rc))
	for _, cc := range rc {
		c, os, err := prepareCmd(cc)
		if err != nil {
			rts = []response{{Status: status(envexec.StatusInternalError), Error: err.Error()}}
			return
		}
		cs = append(cs, c)
		copyOutSets = append(copyOutSets, os)
	}
	g := envexec.Group{
		CgroupPool:      cgroupPool,
		EnvironmentPool: envPool,

		Cmd:   cs,
		Pipes: p,
	}
	results, err := g.Run()
	if err != nil {
		rts = []response{{Status: status(envexec.StatusInternalError), Error: err.Error()}}
		return
	}
	rts = make([]response, 0, len(results))
	for i, result := range results {
		var res response
		res.Status = status(result.Status)
		res.ExitStatus = result.ExitStatus
		res.Error = result.Error
		res.Time = uint64(result.Time)
		res.Memory = uint64(result.Memory)
		res.Files = make(map[string]string)
		res.FileIDs = make(map[string]string)

		for name, fi := range result.Files {
			b, err := fi.Content()
			if err != nil {
				res.Status = status(envexec.StatusFileError)
				res.Error = err.Error()
				return
			}
			if copyOutSets[i][name] {
				res.Files[name] = string(b)
			} else {
				id, err := fs.Add(name, b)
				if err != nil {
					res.Status = status(envexec.StatusFileError)
					res.Error = err.Error()
					return
				}
				res.FileIDs[name] = id
			}
		}
		rts = append(rts, res)
	}
	return
}

func prepareCmd(rc cmd) (*envexec.Cmd, map[string]bool, error) {
	files, pipeFileName, err := prepareCmdFiles(rc.Files)
	if err != nil {
		return nil, nil, err
	}
	copyIn, err := prepareCopyIn(rc.CopyIn)
	if err != nil {
		return nil, nil, err
	}

	copyOutSet := make(map[string]bool)
	// pipe default copyout
	for k := range pipeFileName {
		copyOutSet[k] = true
	}
	copyOut := make([]string, 0, len(rc.CopyOut)+len(rc.CopyOutCached))
	for _, fn := range rc.CopyOut {
		if !pipeFileName[fn] {
			copyOut = append(copyOut, fn)
		}
		copyOutSet[fn] = true
	}
	for _, fn := range rc.CopyOutCached {
		if !pipeFileName[fn] {
			copyOut = append(copyOut, fn)
		} else {
			delete(copyOutSet, fn)
		}
	}

	w := &waiter{
		timeLimit:     time.Duration(rc.CPULimit),
		realTimeLimit: time.Duration(rc.RealCPULimit),
	}

	return &envexec.Cmd{
		Args:        rc.Args,
		Env:         rc.Env,
		Files:       files,
		MemoryLimit: runner.Size(rc.MemoryLimit),
		ProcLimit:   rc.ProcLimit,
		CopyIn:      copyIn,
		CopyOut:     copyOut,
		CopyOutDir:  path.Join(*dir, rc.CopyOutDir),
		Waiter:      w.Wait,
	}, copyOutSet, nil
}

func preparePipeMapping(pm []pipeMap) []*envexec.Pipe {
	rt := make([]*envexec.Pipe, 0, len(pm))
	for _, p := range pm {
		rt = append(rt, &envexec.Pipe{
			In:  envexec.PipeIndex{Index: p.In.Index, Fd: p.In.Fd},
			Out: envexec.PipeIndex{Index: p.Out.Index, Fd: p.Out.Fd},
		})
	}
	return rt
}

func prepareCopyIn(cf map[string]cmdFile) (map[string]file.File, error) {
	rt := make(map[string]file.File)
	for name, f := range cf {
		pcf, err := prepareCmdFile(&f)
		if err != nil {
			return nil, err
		}
		fi, ok := pcf.(file.File)
		if !ok {
			return nil, fmt.Errorf("pipe type cannot be used for copyIn %s", name)
		}
		rt[name] = fi
	}
	return rt, nil
}

func prepareCmdFiles(files []*cmdFile) ([]interface{}, map[string]bool, error) {
	rt := make([]interface{}, 0, len(files))
	pipeFileName := make(map[string]bool)
	for _, f := range files {
		cf, err := prepareCmdFile(f)
		if err != nil {
			return nil, nil, err
		}
		rt = append(rt, cf)
		if t, ok := cf.(envexec.PipeCollector); ok {
			pipeFileName[t.Name] = true
		}
	}
	return rt, pipeFileName, nil
}

func prepareCmdFile(f *cmdFile) (interface{}, error) {
	switch {
	case f == nil:
		return nil, nil
	case f.Src != nil:
		return file.NewLocalFile(*f.Src, *f.Src), nil
	case f.Content != nil:
		return file.NewMemFile("file", []byte(*f.Content)), nil
	case f.FileID != nil:
		fd := fs.Get(*f.FileID)
		if fd == nil {
			return nil, fmt.Errorf("file not exists for %v", *f.FileID)
		}
		return fd, nil
	case f.Max != nil && f.Name != nil:
		return envexec.PipeCollector{Name: *f.Name, SizeLimit: *f.Max}, nil
	default:
		return nil, fmt.Errorf("file is not valid for cmd")
	}
}

func submitRequest(req *request) <-chan []response {
	ch := make(chan []response, 1)
	workCh <- workRequest{
		request:  req,
		resultCh: ch,
	}
	return ch
}

func workerShutdown() {
	workStopOnce.Do(func() {
		workCtxCancel()
		workWg.Wait()
	})
}
