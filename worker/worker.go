package worker

import (
	"context"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/filestore"
)

const maxWaiting = 512

// EnvironmentPool defines pools for environment to be used to execute commands
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
	OpenFileLimit         uint64
	ExecObserver          func(Response)
}

// Worker defines interface for executor
type Worker interface {
	Start()
	Submit(context.Context, *Request) (<-chan Response, <-chan struct{})
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
	openFileLimit         uint64

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
	started  chan<- struct{}
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
		openFileLimit:         conf.OpenFileLimit,
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
func (w *worker) Submit(ctx context.Context, req *Request) (<-chan Response, <-chan struct{}) {
	ch := make(chan Response, 1)
	started := make(chan struct{})
	select {
	case w.workCh <- workRequest{
		Request:  req,
		Context:  ctx,
		started:  started,
		resultCh: ch,
	}:
	default:
		close(started)
		ch <- Response{
			RequestID: req.RequestID,
			Error:     fmt.Errorf("worker queue is full"),
		}
	}
	return ch, started
}

// Execute will execute the request in new goroutine (bypass the parallelism limit)
func (w *worker) Execute(ctx context.Context, req *Request) <-chan Response {
	ch := make(chan Response, 1)
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ch <- w.workDoCmd(ctx, req)
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
			close(req.started)

			select {
			case <-req.Context.Done():
				req.resultCh <- Response{
					RequestID: req.RequestID,
					Error:     fmt.Errorf("cancelled before execute"),
				}
			default:
				req.resultCh <- w.workDoCmd(req.Context, req.Request)
			}

		case <-w.done:
			return
		}
	}
}

func (w *worker) workDoCmd(ctx context.Context, req *Request) Response {
	var rt Response
	if len(req.Cmd) == 1 {
		rt = w.workDoSingle(ctx, req.Cmd[0])
	} else {
		rt = w.workDoGroup(ctx, req.Cmd, req.PipeMapping)
	}
	rt.RequestID = req.RequestID
	if w.execObserver != nil {
		w.execObserver(rt)
	}
	return rt
}

func (w *worker) workDoSingle(ctx context.Context, rc Cmd) (rt Response) {
	c, err := w.prepareCmd(rc)
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
		Cmd:          c,
		NewStoreFile: w.fs.New,
	}
	result, err := s.Run(ctx)
	if err != nil {
		rt.Error = err
		return
	}
	res := w.convertResult(result, rc)
	rt.Results = []Result{res}
	return
}

func (w *worker) workDoGroup(ctx context.Context, rc []Cmd, pm []PipeMap) (rt Response) {
	var rts []Result
	cs := make([]*envexec.Cmd, 0, len(rc))
	for _, cc := range rc {
		c, err := w.prepareCmd(cc)
		if err != nil {
			rt.Error = err
			return
		}
		cs = append(cs, c)
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
		Cmd:          cs,
		Pipes:        pm,
		NewStoreFile: w.fs.New,
	}
	results, err := g.Run(ctx)
	if err != nil {
		rt.Error = err
		return
	}
	rts = make([]Result, 0, len(results))
	for i, result := range results {
		res := w.convertResult(result, rc[i])
		rts = append(rts, res)
	}
	rt.Results = rts
	return
}

func (w *worker) convertResult(result envexec.Result, cmd Cmd) (res Result) {
	res.Status = result.Status
	res.ExitStatus = result.ExitStatus
	res.Error = result.Error
	res.Time = result.Time
	res.RunTime = result.RunTime
	res.Memory = result.Memory
	res.FileError = result.FileError
	res.Files = make(map[string]*os.File)
	res.FileIDs = make(map[string]string)

	// Fix TLE due to context cancel
	if res.Status == envexec.StatusTimeLimitExceeded && res.ExitStatus != 0 &&
		res.Time < cmd.CPULimit && res.RunTime < cmd.ClockLimit {
		res.Status = envexec.StatusSignalled
	}

	copyOutCachedSet := make(map[string]bool, len(cmd.CopyOutCached))
	for _, f := range cmd.CopyOutCached {
		copyOutCachedSet[f.Name] = true
	}

	for name, b := range result.Files {
		if !copyOutCachedSet[name] {
			res.Files[name] = b
			continue
		}
		id, err := w.fs.Add(name, b.Name())
		if err != nil {
			res.Status = envexec.StatusFileError
			res.Error = err.Error()
			return
		}
		res.FileIDs[name] = id
		b.Close()
	}
	return res
}

func (w *worker) prepareCmd(rc Cmd) (*envexec.Cmd, error) {
	files, pipeFileName, err := w.prepareCmdFiles(rc.Files)
	if err != nil {
		return nil, err
	}
	copyIn, err := w.prepareCopyIn(rc.CopyIn)
	if err != nil {
		return nil, err
	}

	copyOut := make([]envexec.CmdCopyOutFile, 0, len(rc.CopyOut)+len(rc.CopyOutCached))
	for _, fn := range rc.CopyOut {
		if !pipeFileName[fn.Name] {
			copyOut = append(copyOut, fn)
		}
	}
	for _, fn := range rc.CopyOutCached {
		if !pipeFileName[fn.Name] {
			copyOut = append(copyOut, fn)
		}
	}

	wait := &waiter{
		tickInterval:   w.timeLimitTickInterval,
		timeLimit:      rc.CPULimit,
		clockTimeLimit: rc.ClockLimit,
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

	outputLimit := rc.OutputLimit
	if outputLimit == 0 {
		outputLimit = w.outputLimit
	}

	openFileLimit := rc.OpenFileLimit
	if openFileLimit == 0 {
		openFileLimit = w.openFileLimit
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
		OutputLimit:       outputLimit,
		ProcLimit:         rc.ProcLimit,
		OpenFileLimit:     openFileLimit,
		CPURateLimit:      rc.CPURateLimit,
		CPUSetLimit:       rc.CPUSetLimit,
		StrictMemoryLimit: rc.StrictMemoryLimit,
		CopyIn:            copyIn,
		CopyOut:           copyOut,
		CopyOutDir:        copyOutDir,
		CopyOutMax:        copyOutMax,
		Waiter:            wait.Wait,
	}, nil
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
		if t, ok := cf.(*envexec.FileCollector); ok {
			pipeFileName[t.Name] = true
		}
	}
	return rt, pipeFileName, nil
}
