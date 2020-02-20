package types

import (
	"time"

	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-sandbox/types"
)

// JudgeTask contains task received from server
type JudgeTask struct {
	Type     string      // defines problem type
	TestData []file.File // test data (potential local)
	Code     SourceCode  // code & code language / answer submit in extra files

	// task parameters
	TimeLimit   time.Duration
	MemoryLimit types.Size
	Extra       map[string]interface{} // extra special parameters
}
