package envexec

import (
	"os"

	"github.com/criyle/go-sandbox/runner"
)

func convertStatus(s runner.Status) Status {
	switch s {
	case runner.StatusNormal:
		return StatusAccepted
	// case runner.StatusSignalled, runner.StatusNonzeroExitStatus:
	// 	return StatusRuntimeError
	case runner.StatusSignalled:
		return StatusSignalled
	case runner.StatusNonzeroExitStatus:
		return StatusNonzeroExitStatus
	case runner.StatusMemoryLimitExceeded:
		return StatusMemoryLimitExceeded
	case runner.StatusTimeLimitExceeded:
		return StatusTimeLimitExceeded
	case runner.StatusOutputLimitExceeded:
		return StatusOutputLimitExceeded
	case runner.StatusDisallowedSyscall:
		return StatusDangerousSyscall
	default:
		return StatusInternalError
	}
}

func getFdArray(fd []*os.File) []uintptr {
	r := make([]uintptr, 0, len(fd))
	for _, f := range fd {
		r = append(r, f.Fd())
	}
	return r
}

func closeFiles(files ...*os.File) {
	for _, f := range files {
		if f == nil {
			continue
		}
		f.Close()
	}
}
