package worker

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/pkg/envexec"
	"github.com/criyle/go-sandbox/runner"
)

const maxWaiting = 512

// Worker defines executor worker
type Worker struct {
	fs        filestore.FileStore
	envPool   envexec.EnvironmentPool
	parallism int
	workDir   string

	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup
	workCh    chan workRequest
	done      chan struct{}
}

type workRequest struct {
	*Request
	context.Context
	resultCh chan<- Response
}

// New creates new worker
func New(fs filestore.FileStore, pool envexec.EnvironmentPool, parallism int, workDir string) *Worker {
	return &Worker{
		fs:        fs,
		envPool:   pool,
		parallism: parallism,
		workDir:   workDir,
	}
}

// Start starts worker loops with given parallism
func (w *Worker) Start() {
	w.startOnce.Do(func() {
		w.workCh = make(chan workRequest, maxWaiting)
		w.done = make(chan struct{})
		w.wg.Add(w.parallism)
		for i := 0; i < w.parallism; i++ {
			go w.loop()
		}
	})
}

// Submit submits a single request
func (w *Worker) Submit(ctx context.Context, req *Request) <-chan Response {
	ch := make(chan Response, 1)
	w.workCh <- workRequest{
		Request:  req,
		Context:  ctx,
		resultCh: ch,
	}
	return ch
}

// Execute will execute the request in new goroutine (bypass the parallism limit)
func (w *Worker) Execute(ctx context.Context, req *Request) <-chan Response {
	ch := make(chan Response, 1)
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		wq := workRequest{
			Request:  req,
			Context:  ctx,
			resultCh: ch,
		}
		w.workDoCmd(wq)
	}()
	return ch
}

// Shutdown waits all worker to finish
func (w *Worker) Shutdown() {
	w.stopOnce.Do(func() {
		close(w.done)
		w.wg.Wait()
	})
}

func (w *Worker) loop() {
	defer w.wg.Done()
	for {
		select {
		case req, ok := <-w.workCh:
			if !ok {
				return
			}
			w.workDoCmd(req)
		case <-w.done:
			return
		}
	}
}

func (w *Worker) workDoCmd(req workRequest) {
	var rt Response
	if len(req.Cmd) == 1 {
		rt = w.workDoSingle(req.Context, req.Cmd[0])
	} else {
		rt = w.workDoGroup(req.Context, req.Cmd, req.PipeMapping)
	}
	rt.RequestID = req.RequestID
	req.resultCh <- rt
}

func (w *Worker) workDoSingle(ctx context.Context, rc Cmd) (rt Response) {
	c, copyOutSet, err := w.prepareCmd(rc)
	if err != nil {
		rt.Error = err
		return
	}
	s := &envexec.Single{
		EnvironmentPool: w.envPool,
		Cmd:             c,
	}
	result, err := s.Run(ctx)
	if err != nil {
		rt.Error = err
		return
	}
	res := w.convertResult(result, copyOutSet)
	rt.Results = []Result{res}
	return
}

func (w *Worker) workDoGroup(ctx context.Context, rc []Cmd, pm []PipeMap) (rt Response) {
	var rts []Result
	p := preparePipeMapping(pm)
	cs := make([]*envexec.Cmd, 0, len(rc))
	copyOutSets := make([]map[string]bool, 0, len(rc))
	for _, cc := range rc {
		c, os, err := w.prepareCmd(cc)
		if err != nil {
			rt.Error = err
			return
		}
		cs = append(cs, c)
		copyOutSets = append(copyOutSets, os)
	}
	g := envexec.Group{
		EnvironmentPool: w.envPool,

		Cmd:   cs,
		Pipes: p,
	}
	results, err := g.Run(ctx)
	if err != nil {
		rt.Error = err
		return
	}
	rts = make([]Result, 0, len(results))
	for i, result := range results {
		res := w.convertResult(result, copyOutSets[i])
		rts = append(rts, res)
	}
	rt.Results = rts
	return
}

func (w *Worker) convertResult(result envexec.Result, copyOutSet map[string]bool) (res Result) {
	res.Status = result.Status
	res.ExitStatus = result.ExitStatus
	res.Error = result.Error
	res.Time = uint64(result.Time)
	res.RunTime = uint64(result.RunTime)
	res.Memory = uint64(result.Memory)
	res.Files = make(map[string][]byte)
	res.FileIDs = make(map[string]string)

	for name, fi := range result.Files {
		b, err := fi.Content()
		if err != nil {
			res.Status = envexec.StatusFileError
			res.Error = err.Error()
			return
		}
		if copyOutSet[name] {
			res.Files[name] = b
		} else {
			id, err := w.fs.Add(name, b)
			if err != nil {
				res.Status = envexec.StatusFileError
				res.Error = err.Error()
				return
			}
			res.FileIDs[name] = id
		}
	}
	return res
}

func (w *Worker) prepareCmd(rc Cmd) (*envexec.Cmd, map[string]bool, error) {
	files, pipeFileName, err := w.prepareCmdFiles(rc.Files)
	if err != nil {
		return nil, nil, err
	}
	copyIn, err := w.prepareCopyIn(rc.CopyIn)
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

	wait := &waiter{
		timeLimit:     time.Duration(rc.CPULimit),
		realTimeLimit: time.Duration(rc.RealCPULimit),
	}

	var copyOutDir string
	if rc.CopyOutDir != "" {
		if path.IsAbs(rc.CopyOutDir) {
			copyOutDir = rc.CopyOutDir
		} else {
			copyOutDir = path.Join(w.workDir, rc.CopyOutDir)
		}
	}

	timeLimit := time.Duration(rc.CPULimit)
	if rc.RealCPULimit > rc.CPULimit {
		timeLimit = time.Duration(rc.RealCPULimit)
	}

	return &envexec.Cmd{
		Args:        rc.Args,
		Env:         rc.Env,
		Files:       files,
		TimeLimit:   timeLimit,
		MemoryLimit: runner.Size(rc.MemoryLimit),
		ProcLimit:   rc.ProcLimit,
		CopyIn:      copyIn,
		CopyOut:     copyOut,
		CopyOutDir:  copyOutDir,
		Waiter:      wait.Wait,
	}, copyOutSet, nil
}

func preparePipeMapping(pm []PipeMap) []*envexec.Pipe {
	rt := make([]*envexec.Pipe, 0, len(pm))
	for _, p := range pm {
		rt = append(rt, &envexec.Pipe{
			In:  envexec.PipeIndex{Index: p.In.Index, Fd: p.In.Fd},
			Out: envexec.PipeIndex{Index: p.Out.Index, Fd: p.Out.Fd},
		})
	}
	return rt
}

func (w *Worker) prepareCopyIn(cf map[string]CmdFile) (map[string]file.File, error) {
	rt := make(map[string]file.File)
	for name, f := range cf {
		if f == nil {
			return nil, fmt.Errorf("nil type cannot be used for copyIn %s", name)
		}
		pcf, err := f.EnvFile(w.fs)
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

func (w *Worker) prepareCmdFiles(files []CmdFile) ([]interface{}, map[string]bool, error) {
	rt := make([]interface{}, 0, len(files))
	pipeFileName := make(map[string]bool)
	for _, f := range files {
		if f == nil {
			rt = append(rt, nil)
			continue
		}
		cf, err := f.EnvFile(w.fs)
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
