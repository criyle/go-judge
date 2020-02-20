package syzojclient

type progressType int

const (
	progressStarted progressType = iota + 1
	progressCompiled
	progressProgress
	progressFinished
	progressReported
)

type taskStatus int

const (
	statusWaiting taskStatus = iota
	statusRunning
	statusDone
	statusFailed
	statusSkipped
)

type errType int

const (
	errSystem errType = iota
	errData
)

type testCaseResultType int

const (
	resultAccepted testCaseResultType = iota + 1
	resultWrongAnswer
	resultPartiallyCorrect
	resultMemoryLimitExceeded
	resultTimeLimitExceeded
	resultOutputLimitExceeded
	resultFileError
	resultRuntimeError
	resultJudgementFailed
	resultInvalidInteraction
)

type result struct {
	TaskID   string       `json:"taskId"`
	Type     progressType `json:"type"`
	Progress progress     `json:"progress"`
}

type progress struct {
	Status  taskStatus `json:"status,omitempty"`
	Message string     `json:"message,omitempty"`

	Error         errType        `json:"error,omitempty"`
	SystemMessage string         `json:"systemMessage,omitempty"`
	Compile       *compileResult `json:"compile,omitempty"`
	Judge         *judgeResult   `json:"judge,omitempty"`
}

type judgeResult struct {
	Subtasks []subtaskResult `json:"subtasks,omitempty"`
}

type subtaskResult struct {
	Score float64      `json:"score,omitempty"`
	Cases []caseResult `json:"cases,omitempty"`
}

type caseResult struct {
	Status taskStatus       `json:"status"`
	Result *testcaseDetails `json:"result,omitempty"`
	Error  string           `json:"errorMessage,omitempty"`
}

type testcaseDetails struct {
	Status        testCaseResultType `json:"type"`
	Time          uint64             `json:"time"`   // ms
	Memory        uint64             `json:"memory"` // kb
	Input         *fileContent       `json:"input,omitempty"`
	Output        *fileContent       `json:"output,omitempty"`
	ScoringRate   float64            `json:"scoringRate,omitempty"`
	UserOutput    *string            `json:"userOutput,omitempty"`
	UserError     *string            `json:"userError,omitempty"`
	SPJMessage    *string            `json:"spjError,omitempty"`
	SystemMessage *string            `json:"systemMessage,omitempty"`
}

type fileContent struct {
	Content string `json:"content"`
	Name    string `json:"name"`
}

type compileResult struct {
	Status  taskStatus `json:"status"`
	Message string     `json:"message"`
}
