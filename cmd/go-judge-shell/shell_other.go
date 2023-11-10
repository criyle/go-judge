//go:build !linux

package main

import "github.com/criyle/go-judge/pb"

func handleSizeChange(sendCh chan<- *pb.StreamRequest) {
}
