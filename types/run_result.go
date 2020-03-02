package types

import (
	"time"

	"github.com/criyle/go-sandbox/runner"
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
	// status
	Status Status

	// score
	ScoringRate float64

	// error if present else empty string
	Error string

	// detail stats
	Time   time.Duration
	Memory runner.Size

	// user stdout stderr
	UserOutput []byte
	UserError  []byte

	// stdin / answer file content
	Input  []byte
	Answer []byte

	// SPJ outputs
	SPJOutput []byte
}
