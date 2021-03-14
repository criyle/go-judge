package worker

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/filestore"
)

const maxWaiting = 512

type EnvironmentPool interface {
	Get() (envexec.Environment, error)
	Put(envexec.Environment)
}

// Config defines worker configuration
type Config struct {
	FileStore             filestore.FileStore
	EnvironmentPool       EnvironmentPool
	Parallelism           int
	WorkDir               string
	TimeLimitTickInterval time.Duration
	ExtraMemoryLimit      envexec.Size
	OutputLimit           envexec.Size
	CopyOutLimit          envexec.Size
	ExecObserver          func(Response)
}

// Worker defines interface for executor
type Worker interface {
	Start()
	Submit(context.Context, *Request) <-chan Response
	Execute(context.Context, *Request) <-chan Response
	Shutdown()
}

// worker defines executor worker
type worker struct {
	fs          filestore.FileStore
	envPool     EnvironmentPool
	parallelism int
	workDir     string

	timeLimitTickInterval time.Duration
	extraMemoryLimit      envexec.Size
	outputLimit           envexec.Size
	copyOutLimit          envexec.Size

	execObserver func(Response)

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
func New(conf Config) Worker {
	return &worker{
		fs:                    conf.FileStore,
		envPool:               conf.EnvironmentPool,
		parallelism:           conf.Parallelism,
		workDir:               conf.WorkDir,
		timeLimitTickInterval: conf.TimeLimitTickInterval,
		extraMemoryLimit:      conf.ExtraMemoryLimit,
		outputLimit:           conf.OutputLimit,
		copyOutLimit:          conf.CopyOutLimit,
		execObserver:          conf.ExecObserver,
	}
}

// Start starts worker loops with given parallelism
func (w *worker) Start() {
	w.startOnce.Do(func() {
		w.workCh = make(chan workRequest, maxWaiting)
		w.done = make(chan struct{})
		w.wg.Add(w.parallelism)
		for i := 0; i < w.parallelism; i++ {
			go w.loop()
		}
	})
}

// Submit submits a single request
func (w *worker) Submit(ctx context.Context, req *Request) <-chan Response {
	ch := make(chan Response, 1)
	w.workCh <- workRequest{
		Request:  req,
		Context:  ctx,
		resultCh: ch,
	}
	return ch
}

// Execute will execute the request in new goroutine (bypass the parallelism limit)
func (w *worker) Execute(ctx context.Context, req *Request) <-chan Response {
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
func (w *worker) Shutdown() {
	w.stopOnce.Do(func() {
		close(w.done)
		w.wg.Wait()
	})
}

func (w *worker) loop() {
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

func (w *worker) workDoCmd(req workRequest) {
	var rt Response
	if len(req.Cmd) == 1 {
		rt = w.workDoSingle(req.Context, req.Cmd[0])
	} else {
		rt = w.workDoGroup(req.Context, req.Cmd, req.PipeMapping)
	}
	rt.RequestID = req.RequestID
	if w.execObserver != nil {
		w.execObserver(rt)
	}
	req.resultCh <- rt
}

func (w *worker) workDoSingle(ctx context.Context, rc Cmd) (rt Response) {
	c, copyOutSet, err := w.prepareCmd(rc)
	if err != nil {
		rt.Error = err
		return
	}
	// prepare environment
	env, err := w.envPool.Get()
	if err != nil {
		return Response{Results: []Result{{
			Status: envexec.StatusInternalError,
			Error:  fmt.Sprintf("failed to get environment %v", err),
		}}}
	}
	defer w.envPool.Put(env)
	c.Environment = env

	s := &envexec.Single{
		Cmd: c,
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

func (w *worker) workDoGroup(ctx context.Context, rc []Cmd, pm []PipeMap) (rt Response) {
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
	for i := range cs {
		env, err := w.envPool.Get()
		if err != nil {
			res := make([]Result, 0, len(cs))
			for range cs {
				res = append(res, Result{
					Status: envexec.StatusInternalError,
					Error:  fmt.Sprintf("failed to get environment %v", err),
				})
			}
			return Response{Results: res}
		}
		defer w.envPool.Put(env)
		cs[i].Environment = env
	}
	g := envexec.Group{
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

func (w *worker) convertResult(result envexec.Result, copyOutSet map[string]bool) (res Result) {
	res.Status = result.Status
	res.ExitStatus = result.ExitStatus
	res.Error = result.Error
	res.Time = result.Time
	res.RunTime = result.RunTime
	res.Memory = result.Memory
	res.Files = make(map[string][]byte)
	res.FileIDs = make(map[string]string)

	for name, b := range result.Files {
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

func (w *worker) prepareCmd(rc Cmd) (*envexec.Cmd, map[string]bool, error) {
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
		tickInterval:  w.timeLimitTickInterval,
		timeLimit:     time.Duration(rc.CPULimit),
		realTimeLimit: time.Duration(rc.ClockLimit),
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
	copyOutMax := w.copyOutLimit
	if rc.CopyOutMax > 0 {
		copyOutMax = envexec.Size(rc.CopyOutMax)
	}

	return &envexec.Cmd{
		Args:              rc.Args,
		Env:               rc.Env,
		Files:             files,
		TTY:               rc.TTY,
		TimeLimit:         timeLimit,
		MemoryLimit:       envexec.Size(rc.MemoryLimit),
		StackLimit:        envexec.Size(rc.StackLimit),
		ExtraMemoryLimit:  w.extraMemoryLimit,
		OutputLimit:       w.outputLimit,
		ProcLimit:         rc.ProcLimit,
		CPURateLimit:      rc.CPURateLimit,
		StrictMemoryLimit: rc.StrictMemoryLimit,
		CopyIn:            copyIn,
		CopyOut:           copyOut,
		CopyOutDir:        copyOutDir,
		CopyOutMax:        copyOutMax,
		Waiter:            wait.Wait,
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

func (w *worker) prepareCopyIn(cf map[string]CmdFile) (map[string]envexec.File, error) {
	rt := make(map[string]envexec.File)
	for name, f := range cf {
		if f == nil {
			return nil, fmt.Errorf("nil type cannot be used for copyIn %s", name)
		}
		pcf, err := f.EnvFile(w.fs)
		if err != nil {
			return nil, err
		}
		rt[name] = pcf
	}
	return rt, nil
}

func (w *worker) prepareCmdFiles(files []CmdFile) ([]envexec.File, map[string]bool, error) {
	rt := make([]envexec.File, 0, len(files))
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
		if t, ok := cf.(*envexec.FilePipeCollector); ok {
			pipeFileName[t.Name] = true
		}
	}
	return rt, pipeFileName, nil
}
