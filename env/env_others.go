//go:build !windows && !linux && !darwin

package env

import (
	"errors"
	"runtime"

	"github.com/criyle/go-judge/env/pool"
	"go.uber.org/zap"
)

func NewBuilder(c Config, logger *zap.Logger) (pool.EnvBuilder, map[string]any, error) {
	return nil, nil, errors.New("environment is not support on this platform" + runtime.GOOS)
}
