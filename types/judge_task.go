package types

import "github.com/criyle/go-judge/file"

// JudgeTask contains task received from server
type JudgeTask struct {
	Type     string      // defines problem type
	TestData []file.File // test data (potential local)
	Code     SourceCode  // code & code language / answer submit in extra files

	// task parameters
	TileLimit   uint64
	MemoryLimit uint64
	Extra       map[string]interface{} // extra special parameters
}
