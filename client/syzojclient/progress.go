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
	Status  taskStatus `json:"status"`
	Message string     `json:"message"`

	Error         *errType       `json:"error"`
	SystemMessage *string        `json:"systemMessage"`
	Compile       *compileResult `json:"compile"`
	Judge         *judgeResult   `json:"judge"`
}

type judgeResult struct {
	Subtasks []subtaskResult `json:"subtasks"`
}

type subtaskResult struct {
	Score *float64     `json:"score"`
	Cases []caseResult `json:"cases"`
}

type caseResult struct {
	Status taskStatus       `json:"status"`
	Result *testcaseDetails `json:"result"`
	Error  *string          `json:"errorMessage"`
}

type testcaseDetails struct {
	Status        testCaseResultType `json:"type"`
	Time          uint64             `json:"time"`   // ms
	Memory        uint64             `json:"memory"` // kb
	Input         *fileContent       `json:"input"`
	Output        *fileContent       `json:"output"`
	ScoringRate   float64            `json:"scoringRate"`
	UserOutput    *string            `json:"userOutput"`
	UserError     *string            `json:"userError"`
	SPJMessage    *string            `json:"spjError"`
	SystemMessage *string            `json:"systemMessage"`
}

type fileContent struct {
	Content string `json:"content"`
	Name    string `json:"name"`
}

type compileResult struct {
	Status  taskStatus `json:"status"`
	Message string     `json:"message"`
}
