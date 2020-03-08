package envexec

// Status defines run task Status return status
type Status int

// Defines run task Status result status
const (
	// not initialized status (as error)
	StatusInvalid Status = iota

	// exit normally
	StatusAccepted
	StatusWrongAnswer
	StatusPartiallyCorrect

	// exit with error
	StatusMemoryLimitExceeded // MLE
	StatusTimeLimitExceeded   // TLE
	StatusOutputLimitExceeded // OLE
	StatusFileError           // FE
	StatusNonzeroExitStatus   // NZS
	StatusSignalled           // SIG
	StatusDangerousSyscall    // DJS

	//StatusRuntimeError        // RE

	// SPJ / interactor error
	StatusJudgementFailed
	StatusInvalidInteraction // interactor signals error

	// internal error including: cgroup init failed, container failed, etc
	StatusInternalError
)

var statusToString = []string{
	"Invalid",
	"Accepted",
	"Wrong Answer",
	"Partially Correct",
	"Memory Limit Exceeded",
	"Time Limit Exceeded",
	"Output Limit Exceeded",
	"File Error",
	//"Runtime Error",
	"Nonzero Exit Status",
	"Signalled",
	"Dangerous Syscall",
	"Judgement Failed",
	"Invalid Interaction",
	"Internal Error",
	"CGroup Error",
	"Container Error",
}

func (s Status) String() string {
	si := int(s)
	if si < 0 || si >= len(statusToString) {
		return statusToString[0] // invalid
	}
	return statusToString[si]
}
