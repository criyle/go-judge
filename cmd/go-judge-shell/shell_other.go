//go:build !linux

package main

import "github.com/criyle/go-judge/cmd/go-judge/stream"

func handleSizeChange(sendCh chan *stream.Request) {
}
