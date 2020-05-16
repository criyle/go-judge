package main

import (
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

func initEnvPool() {
	b := macsandbox.NewBuilder("", defaultRead, defaultWrite, *netShare)
	printLog("created mac sandbox at", "")
	envPool = pool.NewPool(b)
}
