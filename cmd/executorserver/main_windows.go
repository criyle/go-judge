package main

import (
	"log"

	"github.com/criyle/go-judge/pkg/pool"
	"github.com/criyle/go-judge/pkg/winc"
)

func initEnvPool() {
	b, err := winc.NewBuilder("")
	if err != nil {
		log.Fatalln("init container", err)
	}
	printLog("created winc builder")
	envPool = pool.NewPool(b)
}
