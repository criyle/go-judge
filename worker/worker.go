package worker

import (
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
}

type workRequest struct {
	*Request
	resultCh chan<- Result
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
		w.wg.Add(w.parallism)
		for i := 0; i < w.parallism; i++ {
			go w.loop()
		}
	})
}

// Submit submits a single request
func (w *Worker) Submit(req *Request) <-chan Result {
	ch := make(chan Result, 1)
	w.workCh <- workRequest{
		Request:  req,
		resultCh: ch,
	}
	return ch
}

// Shutdown waits all worker to finish
func (w *Worker) Shutdown() {
	w.stopOnce.Do(func() {
		close(w.workCh)
		w.wg.Wait()
	})
}

func (w *Worker) loop() {
	defer w.wg.Done()
	for {
		req, ok := <-w.workCh
		if !ok {
			return
		}
		w.workDoCmd(req)
	}
}

func (w *Worker) workDoCmd(req workRequest) {
	var rt Result
	if len(req.Cmd) == 1 {
		rt = w.workDoSingle(req.Cmd[0])
	} else {
		rt = w.workDoGroup(req.Cmd, req.PipeMapping)
	}
	rt.RequestID = req.RequestID
	req.resultCh <- rt
}

func (w *Worker) workDoSingle(rc Cmd) (rt Result) {
	c, copyOutSet, err := w.prepareCmd(rc)
	if err != nil {
		rt.Error = err
		return
	}
	s := &envexec.Single{
		EnvironmentPool: w.envPool,
		Cmd:             c,
	}
	result, err := s.Run()
	if err != nil {
		rt.Error = err
		return
	}
	res := w.convertResult(result, copyOutSet)
	rt.Response = []Response{res}
	return
}

func (w *Worker) workDoGroup(rc []Cmd, pm []PipeMap) (rt Result) {
	var rts []Response
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
	results, err := g.Run()
	if err != nil {
		rt.Error = err
		return
	}
	rts = make([]Response, 0, len(results))
	for i, result := range results {
		res := w.convertResult(result, copyOutSets[i])
		rts = append(rts, res)
	}
	rt.Response = rts
	return
}

func (w *Worker) convertResult(result envexec.Result, copyOutSet map[string]bool) (res Response) {
	res.Status = Status(result.Status)
	res.ExitStatus = result.ExitStatus
	res.Error = result.Error
	res.Time = uint64(result.Time)
	res.Memory = uint64(result.Memory)
	res.Files = make(map[string]string)
	res.FileIDs = make(map[string]string)

	for name, fi := range result.Files {
		b, err := fi.Content()
		if err != nil {
			res.Status = Status(envexec.StatusFileError)
			res.Error = err.Error()
			return
		}
		if copyOutSet[name] {
			res.Files[name] = string(b)
		} else {
			id, err := w.fs.Add(name, b)
			if err != nil {
				res.Status = Status(envexec.StatusFileError)
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
		pcf, err := w.prepareCmdFile(&f)
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

func (w *Worker) prepareCmdFiles(files []*CmdFile) ([]interface{}, map[string]bool, error) {
	rt := make([]interface{}, 0, len(files))
	pipeFileName := make(map[string]bool)
	for _, f := range files {
		cf, err := w.prepareCmdFile(f)
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

func (w *Worker) prepareCmdFile(f *CmdFile) (interface{}, error) {
	switch {
	case f == nil:
		return nil, nil
	case f.Src != nil:
		return file.NewLocalFile(*f.Src, *f.Src), nil
	case f.Content != nil:
		return file.NewMemFile("file", []byte(*f.Content)), nil
	case f.FileID != nil:
		fd := w.fs.Get(*f.FileID)
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
