package main

import (
	"github.com/criyle/go-judge/pkg/envexec"
	"github.com/criyle/go-judge/pkg/macsandbox"
	"github.com/criyle/go-judge/pkg/pool"
)

var defaultRead = []string{
	"/",
}

var defaultWrite = []string{
	"/tmp",
	"/dev/null",
	"/var/tmp",
}

func newEnvPool() envexec.EnvironmentPool {
	b := macsandbox.NewBuilder("", defaultRead, defaultWrite, *netShare)
	printLog("created mac sandbox at", "")
	return pool.NewPool(b)
}
