package problem

import (
	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-judge/types"
)

// Builder builds problem specs from file
type Builder interface {
	Build([]file.File) (types.ProblemConfig, error)
}
