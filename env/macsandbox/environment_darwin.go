package macsandbox

import (
	"context"
	"os"
	"path"
	"syscall"
	"time"

	"github.com/criyle/go-judge/env/pool"
	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-sandbox/pkg/forkexec"
	"github.com/criyle/go-sandbox/pkg/rlimit"
	"github.com/criyle/go-sandbox/runner"
	"golang.org/x/sys/unix"
)

const outputLimit = 256 << 20 // 256M

var _ pool.Environment = &environment{}

type environment struct {
	profile string
	wdPath  string
	wd      *os.File
}

func (e *environment) Execve(c context.Context, param envexec.ExecveParam) (envexec.Process, error) {
	sTime := time.Now()
	rLimits := rlimit.RLimits{
		CPU:      uint64(param.Limit.Time.Truncate(time.Second)/time.Second) + 1,
		Data:     param.Limit.Memory.Byte(),
		FileSize: outputLimit,
		Stack:    param.Limit.Stack.Byte(),
	}

	ch := &forkexec.Runner{
		Args:           param.Args,
		Env:            param.Env,
		Files:          param.Files,
		WorkDir:        e.wdPath,
		RLimits:        rLimits.PrepareRLimit(),
		SandboxProfile: e.profile,
	}

	pid, err := ch.Start()
	if err != nil {
		return nil, err
	}

	p := &process{
		pid:  pid,
		done: make(chan struct{}),
	}

	go func() {
		defer close(p.done)

		mTime := time.Now()

		// handle cancel
		ctx, cancel := context.WithCancel(c)
		defer cancel()

		go func() {
			<-ctx.Done()
			killAll(pid)
		}()

		// collect potential zombies
		defer func() {
			killAll(pid)
			collectZombie(pid)
		}()

		var (
			wstatus syscall.WaitStatus
			rusage  syscall.Rusage
		)
		for {
			_, err = syscall.Wait4(pid, &wstatus, 0, &rusage)
			if err == syscall.EINTR {
				continue
			}
			if err != nil {
				p.result.Error = err.Error()
				p.result.Status = runner.StatusRunnerError
				return
			}
			fTime := time.Now()
			p.result = runner.Result{
				Status:      runner.StatusNormal,
				Time:        time.Duration(rusage.Utime.Nano()),
				Memory:      runner.Size(rusage.Maxrss), // seems MacOS uses bytes instead of kb
				SetUpTime:   mTime.Sub(sTime),
				RunningTime: fTime.Sub(mTime),
			}
			if p.result.Time > param.Limit.Time {
				p.result.Status = runner.StatusTimeLimitExceeded
			}
			if p.result.Memory > param.Limit.Memory {
				p.result.Status = runner.StatusMemoryLimitExceeded
			}

			switch {
			case wstatus.Exited():
				if status := wstatus.ExitStatus(); status != 0 {
					p.result.Status = runner.StatusNonzeroExitStatus
					return
				}
				return

			case wstatus.Signaled():
				sig := wstatus.Signal()
				switch sig {
				case unix.SIGXCPU, unix.SIGKILL:
					p.result.Status = runner.StatusTimeLimitExceeded
				case unix.SIGXFSZ:
					p.result.Status = runner.StatusOutputLimitExceeded
				case unix.SIGSYS:
					p.result.Status = runner.StatusDisallowedSyscall
				default:
					p.result.Status = runner.StatusSignalled
				}
				p.result.ExitStatus = int(sig)
				return
			}
		}
	}()

	return p, nil
}

func (e *environment) WorkDir() *os.File {
	return e.wd
}

func (e *environment) Open(p string, flags int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(path.Join(e.wdPath, p), flags, perm)
}

func (e *environment) MkdirAll(p string, perm os.FileMode) error {
	return os.MkdirAll(path.Join(e.wdPath, p), perm)
}

func (e *environment) Destroy() error {
	e.wd.Close()
	return os.RemoveAll(e.wdPath)
}

func (e *environment) Reset() error {
	return removeContents(e.wdPath)
}

// removeContents delete content of a directory
func removeContents(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()

	names, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}

	for _, name := range names {
		err = os.RemoveAll(path.Join(dir, name))
		if err != nil {
			return err
		}
	}
	return nil
}

func killAll(pid int) {
	syscall.Kill(pid, syscall.SIGKILL)
}

// collect died child processes
func collectZombie(pgid int) {
	var wstatus syscall.WaitStatus
	for {
		if _, err := syscall.Wait4(-pgid, &wstatus, syscall.WNOHANG, nil); err != syscall.EINTR && err != nil {
			break
		}
	}
}
