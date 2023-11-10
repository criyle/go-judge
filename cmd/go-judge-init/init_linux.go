package main

import (
	"os"

	"github.com/criyle/go-sandbox/container"
)

func main() {
	container.Init()
	os.Exit(2)
}
