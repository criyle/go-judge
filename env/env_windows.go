package env

import (
	"github.com/criyle/go-judge/env/pool"
	"github.com/criyle/go-judge/env/winc"
	"go.uber.org/zap"
)

// NewBuilder build a environment builder
func NewBuilder(c Config, logger *zap.Logger) (pool.EnvBuilder, map[string]any, error) {
	b, err := winc.NewBuilder("")
	if err != nil {
		return nil, nil, err
	}
	logger.Info("created winc builder")
	return b, map[string]any{}, nil
}
