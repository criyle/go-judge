package problem

import (
	"github.com/criyle/go-judge/file"
)

// Builder builds problem specs from file
type Builder interface {
	Build([]file.File) (Config, error)
}
