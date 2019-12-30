package types

// RunTaskStatus defines success / fail
type RunTaskStatus int

// Success / Fail
const (
	RunTaskSucceeded RunTaskStatus = iota + 1
	RunTaskFailed
)

// RunTaskResult return the result for run task RPC
type RunTaskResult struct {
	// status
	Status RunTaskStatus // done / failed

	// compile result
	Compile *CompileResult

	// exec result
	Exec *ExecResult
}

// CompileResult returns result for compile tasks
type CompileResult struct {
	Exec  *CompiledExec // contains exec if success
	Error string        // error message if failed
}

// ExecResult returns result for exec tasks
type ExecResult struct {
	// score
	ScoringRate float64

	// error if present else empty string
	Error string

	// detail stats
	Time   uint64 // ms
	Memory uint64 // kb

	// user stdout stderr
	UserOutput []byte
	UserError  []byte

	// stdin / answer file content
	Input  []byte
	Answer []byte

	// SPJ outputs
	SPJOutput []byte
}
