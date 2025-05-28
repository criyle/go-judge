package env

import (
	"github.com/criyle/go-judge/env/macsandbox"
	"github.com/criyle/go-judge/env/pool"
	"go.uber.org/zap"
)

var defaultRead = []string{
	"/",
}

var defaultWrite = []string{
	"/tmp",
	"/dev/null",
	"/var/tmp",
}

// NewBuilder build a environment builder
func NewBuilder(c Config, logger *zap.Logger) (pool.EnvBuilder, map[string]any, error) {
	b := macsandbox.NewBuilder("", defaultRead, defaultWrite, c.NetShare)
	logger.Info("created mac sandbox")
	return b, map[string]any{}, nil
}
