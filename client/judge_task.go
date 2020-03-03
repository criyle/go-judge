package client

import (
	"time"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-sandbox/runner"
)

// JudgeTask contains task received from server
type JudgeTask struct {
	Type     string          // defines problem type
	TestData []file.File     // test data (potential local)
	Code     file.SourceCode // code & code language / answer submit in extra files

	// task parameters
	TimeLimit   time.Duration
	MemoryLimit runner.Size
	Extra       map[string]interface{} // extra special parameters
}
