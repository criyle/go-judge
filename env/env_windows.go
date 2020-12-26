package env

import (
	"github.com/criyle/go-judge/env/pool"
	"github.com/criyle/go-judge/env/winc"
)

// NewBuilder build a environment builder
func NewBuilder(c Config) (pool.EnvBuilder, error) {
	b, err := winc.NewBuilder("")
	if err != nil {
		return nil, err
	}
	c.Info("created winc builder")
	return b, nil
}
