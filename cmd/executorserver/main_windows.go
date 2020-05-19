package main

import (
	"log"

	"github.com/criyle/go-judge/pkg/envexec"
	"github.com/criyle/go-judge/pkg/pool"
	"github.com/criyle/go-judge/pkg/winc"
)

func newEnvPool() envexec.EnvironmentPool {
	b, err := winc.NewBuilder("")
	if err != nil {
		log.Fatalln("init container", err)
	}
	printLog("created winc builder")
	return pool.NewPool(b)
}
