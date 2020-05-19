package env

import (
	"github.com/criyle/go-judge/pkg/pool"
	"github.com/criyle/go-judge/pkg/winc"
)

// NewBuilder build a environment builder
func NewBuilder(cinitPath, mountConf, tmpFsConf string, netShare bool, printLog func(v ...interface{})) (pool.EnvBuilder, error) {
	b, err := winc.NewBuilder("")
	if err != nil {
		return nil, err
	}
	printLog("created winc builder")
	return b, nil
}
