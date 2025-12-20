package winc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/criyle/go-judge/env/pool"
	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-sandbox/runner"
	"golang.org/x/sys/windows"
)

var _ pool.Environment = &Environment{}

var (
	errFileCount = errors.New("windows requires std handle to be 3")
	errArgs      = errors.New("executable name is required")
)

// Environment implements envexec.Environment interface
type Environment struct {
	root  string
	tmp   string
	token windows.Token
	sd    *windows.SECURITY_DESCRIPTOR
}

// Execve implements windows sandbox ..
func (e *Environment) Execve(ctx context.Context, param envexec.ExecveParam) (proc envexec.Process, err error) {
	startTime := time.Now()
	if len(param.Files) != 3 {
		return nil, errFileCount
	}
	if len(param.Args) == 0 {
		return nil, errArgs
	}

	argv0, err := joinExeDirAndFName(e.root, param.Args[0])
	if err != nil {
		return nil, err
	}
	param.Args[0] = argv0

	// cleanUp defines clean up functions to call when error happens
	var cleanUp []func()
	handle := func(f func()) {
		cleanUp = append(cleanUp, f)
	}
	defer func() {
		if err != nil {
			for i := len(cleanUp) - 1; i >= 0; i-- {
				cleanUp[i]()
			}
		}
	}()

	// create desktop
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return nil, err
	}

	deskName := fmt.Sprintf("winc_%08x_%s", windows.GetCurrentProcessId(), hex.EncodeToString(random))
	deskNameW, err := syscall.UTF16PtrFromString(deskName)
	if err != nil {
		return nil, err
	}

	sa := windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: e.sd,
	}

	newDesk, err := CreateDesktop(deskNameW, nil, 0, 0, GENERIC_ALL, &sa)
	if err != nil {
		return nil, err
	}
	handle(func() { CloseDesktop(newDesk) })

	// create job object
	hJob, err := createJobObject(param.Limit)
	if err != nil {
		return nil, err
	}
	handle(func() { windows.CloseHandle(hJob) })

	// create IOCP
	ioPort, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 1)
	if err != nil {
		return nil, err
	}
	handle(func() { windows.CloseHandle(ioPort) })

	completionPort := &JOBOBJECT_ASSOCIATE_COMPLETION_PORT{
		CompletionKey:  hJob,
		CompletionPort: ioPort,
	}
	if _, err := windows.SetInformationJobObject(hJob, windows.JobObjectAssociateCompletionPortInformation,
		uintptr(unsafe.Pointer(completionPort)), uint32(unsafe.Sizeof(*completionPort))); err != nil {
		return nil, err
	}

	cmdLine := makeCmdLine(param.Args)
	cmdLineW, err := syscall.UTF16PtrFromString(cmdLine)
	if err != nil {
		return nil, err
	}
	dirW, err := syscall.UTF16PtrFromString(e.root)
	if err != nil {
		return nil, err
	}

	var startupInfo syscall.StartupInfo
	startupInfo.Cb = uint32(unsafe.Sizeof(startupInfo))
	startupInfo.Flags |= windows.STARTF_USESTDHANDLES // STARTF_FORCEOFFFEEDBACK
	startupInfo.Desktop = deskNameW

	var env []string
	env = append(env, param.Env...)
	env = append(env, "TMP="+e.tmp)
	env = append(env, "TEMP="+e.tmp)
	env = append(env, "LocalAppData="+e.tmp)
	eb := createEnvBlock(env)

	// do create process
	var processInfo syscall.ProcessInformation
	err = func() error {
		syscall.ForkLock.Lock()
		defer syscall.ForkLock.Unlock()

		curProc := windows.CurrentProcess()
		fd := make([]windows.Handle, len(param.Files))
		for i := range param.Files {
			if param.Files[i] <= 0 {
				continue
			}
			if err := windows.DuplicateHandle(curProc, windows.Handle(param.Files[i]),
				curProc, &fd[i], 0, true, syscall.DUPLICATE_SAME_ACCESS); err != nil {
				return err
			}
			defer windows.CloseHandle(fd[i])
		}
		startupInfo.StdInput = syscall.Handle(fd[0])
		startupInfo.StdOutput = syscall.Handle(fd[1])
		startupInfo.StdErr = syscall.Handle(fd[2])

		return syscall.CreateProcessAsUser(syscall.Token(e.token), nil, cmdLineW, nil, nil, true,
			windows.CREATE_NEW_PROCESS_GROUP|windows.CREATE_NEW_CONSOLE|
				windows.CREATE_SUSPENDED|windows.CREATE_UNICODE_ENVIRONMENT,
			eb, dirW, &startupInfo, &processInfo)
	}()

	if processInfo.Process > 0 {
		handle(func() { syscall.CloseHandle(processInfo.Process) })
	}
	if processInfo.Thread > 0 {
		handle(func() { syscall.CloseHandle(processInfo.Thread) })
	}

	if err != nil {
		return nil, err
	}

	// assign process to job object
	if err := windows.AssignProcessToJobObject(hJob, windows.Handle(processInfo.Process)); err != nil {
		return nil, err
	}

	setUpTime := time.Now()
	// resume thread
	if _, err := windows.ResumeThread(windows.Handle(processInfo.Thread)); err != nil {
		return nil, err
	}
	syscall.CloseHandle(processInfo.Thread)

	done := make(chan struct{})

	procSet := &process{
		done:     done,
		hProcess: windows.Handle(processInfo.Process),
		hJob:     hJob,
	}

	// wait for ctx to terminate
	go func() {
		select {
		case <-ctx.Done():
			windows.TerminateJobObject(hJob, 1)
		case <-done:
		}
	}()

	// wait for job object to finish
	go func() {
		defer CloseDesktop(newDesk)
		defer syscall.CloseHandle(processInfo.Process)
		defer windows.CloseHandle(hJob)
		defer windows.CloseHandle(ioPort)
		defer close(done)

		var (
			qty        uint32
			key        uintptr
			overlapped *windows.Overlapped
		)
		result := runner.Result{
			Status: runner.StatusNormal,
		}

	loop:
		for {
			err = windows.GetQueuedCompletionStatus(ioPort, &qty, &key, &overlapped, windows.INFINITE)
			if err != nil {
				procSet.result = runner.Result{Status: runner.StatusRunnerError, Error: err.Error()}
				return
			}
			switch qty {
			case JOB_OBJECT_MSG_ACTIVE_PROCESS_ZERO:
				break loop

			case JOB_OBJECT_MSG_END_OF_JOB_TIME, JOB_OBJECT_MSG_END_OF_PROCESS_TIME:
				result.Status = runner.StatusTimeLimitExceeded
				windows.TerminateJobObject(hJob, 0)

			case JOB_OBJECT_MSG_ACTIVE_PROCESS_LIMIT:
				result.Status = runner.StatusMemoryLimitExceeded
				windows.TerminateJobObject(hJob, 0)

			// case JOB_OBJECT_MSG_ABNORMAL_EXIT_PROCESS:
			// 	result.Status = runner.StatusNonzeroExitStatus
			// 	windows.TerminateJobObject(hJob, 0)

			case JOB_OBJECT_MSG_PROCESS_MEMORY_LIMIT, JOB_OBJECT_MSG_JOB_MEMORY_LIMIT:
				result.Status = runner.StatusMemoryLimitExceeded
				windows.TerminateJobObject(hJob, 0)
				// JOB_OBJECT_MSG_NEW_PROCESS, JOB_OBJECT_MSG_EXIT_PROCESS
			}
		}
		// collect exit status
		if _, err := windows.WaitForSingleObject(windows.Handle(processInfo.Process), windows.INFINITE); err != nil {
			procSet.result = runner.Result{Status: runner.StatusRunnerError, Error: err.Error()}
			return
		}

		var exitCode uint32
		if err := windows.GetExitCodeProcess(windows.Handle(processInfo.Process), &exitCode); err != nil {
			procSet.result = runner.Result{Status: runner.StatusRunnerError, Error: err.Error()}
			return
		}
		if exitCode != 0 {
			result.Status = runner.StatusNonzeroExitStatus
		}
		result.ExitStatus = int(exitCode)

		// collect usage
		t, m, err := getJobObjectUsage(hJob)
		if err != nil {
			procSet.result = runner.Result{Status: runner.StatusRunnerError, Error: err.Error()}
			return
		}
		result.Time = t
		result.Memory = m
		result.SetUpTime = setUpTime.Sub(startTime)
		result.RunningTime = time.Since(setUpTime)
		procSet.result = result
	}()

	return procSet, nil
}

func (e *Environment) Open(params []envexec.OpenParam) ([]envexec.OpenResult, error) {
	results := make([]envexec.OpenResult, len(params))
	for i, param := range params {
		fullPath := filepath.Join(e.root, param.Path)
		if param.MkdirAll {
			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				results[i] = envexec.OpenResult{Err: err}
				continue
			}
		}
		f, err := os.OpenFile(fullPath, param.Flag, param.Perm)
		if err != nil {
			results[i] = envexec.OpenResult{Err: err}
		} else {
			results[i] = envexec.OpenResult{File: f}
		}
	}
	return results, nil
}

func (e *Environment) Symlink(params []envexec.SymlinkParam) ([]error, error) {
	errs := make([]error, len(params))
	for i, l := range params {
		fullLinkPath := filepath.Join(e.root, l.LinkPath)
		if err := os.Symlink(l.Target, fullLinkPath); err != nil {
			errs[i] = err
		} else {
			errs[i] = nil
		}
	}
	return errs, nil
}

// Destroy destroys the environment
func (e *Environment) Destroy() error {
	// error is ignorable for destroy operation
	os.RemoveAll(e.root)
	return os.RemoveAll(e.tmp)
}

// Reset remove all files in root directory
func (e *Environment) Reset() error {
	if err := removeContents(e.tmp); err != nil {
		return err
	}
	return removeContents(e.root)
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
		err = os.RemoveAll(filepath.Join(dir, name))
		if err != nil {
			return err
		}
	}
	return nil
}

// From syscall/exec_windows.go

// makeCmdLine builds a command line out of args by escaping "special"
// characters and joining the arguments with spaces.
func makeCmdLine(args []string) string {
	var s string
	for _, v := range args {
		if s != "" {
			s += " "
		}
		s += syscall.EscapeArg(v)
	}
	return s
}

// createEnvBlock converts an array of environment strings into
// the representation required by CreateProcess: a sequence of NUL
// terminated strings followed by a nil.
// Last bytes are two UCS-2 NULs, or four NUL bytes.
func createEnvBlock(envv []string) *uint16 {
	if len(envv) == 0 {
		return &utf16.Encode([]rune("\x00\x00"))[0]
	}
	length := 0
	for _, s := range envv {
		length += len(s) + 1
	}
	length++

	b := make([]byte, length)
	i := 0
	for _, s := range envv {
		l := len(s)
		copy(b[i:i+l], []byte(s))
		copy(b[i+l:i+l+1], []byte{0})
		i = i + l + 1
	}
	copy(b[i:i+1], []byte{0})

	return &utf16.Encode([]rune(string(b)))[0]
}

func isSlash(c uint8) bool {
	return c == '\\' || c == '/'
}

func normalizeDir(dir string) (name string, err error) {
	ndir, err := syscall.FullPath(dir)
	if err != nil {
		return "", err
	}
	if len(ndir) > 2 && isSlash(ndir[0]) && isSlash(ndir[1]) {
		// dir cannot have \\server\share\path form
		return "", syscall.EINVAL
	}
	return ndir, nil
}

func volToUpper(ch int) int {
	if 'a' <= ch && ch <= 'z' {
		ch += 'A' - 'a'
	}
	return ch
}

func joinExeDirAndFName(dir, p string) (name string, err error) {
	if len(p) == 0 {
		return "", syscall.EINVAL
	}
	if len(p) > 2 && isSlash(p[0]) && isSlash(p[1]) {
		// \\server\share\path form
		return p, nil
	}
	if len(p) > 1 && p[1] == ':' {
		// has drive letter
		if len(p) == 2 {
			return "", syscall.EINVAL
		}
		if isSlash(p[2]) {
			return p, nil
		}
		d, err := normalizeDir(dir)
		if err != nil {
			return "", err
		}
		if volToUpper(int(p[0])) == volToUpper(int(d[0])) {
			return syscall.FullPath(d + "\\" + p[2:])
		}
		return syscall.FullPath(p)

	}
	// no drive letter
	d, err := normalizeDir(dir)
	if err != nil {
		return "", err
	}
	if isSlash(p[0]) {
		return syscall.FullPath(d[:2] + p)
	}
	return syscall.FullPath(d + "\\" + p)
}
