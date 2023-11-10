package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"github.com/criyle/go-judge/pb"
)

func handleSizeChange(sendCh chan<- *pb.StreamRequest) {
	// pump resize
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			winSize, err := pty.GetsizeFull(os.Stdin)
			if err != nil {
				log.Println("get win size", err)
				return
			}
			sendCh <- &pb.StreamRequest{
				Request: &pb.StreamRequest_ExecResize{
					ExecResize: &pb.StreamRequest_Resize{
						Name: "stdin",
						Rows: uint32(winSize.Rows),
						Cols: uint32(winSize.Cols),
						X:    uint32(winSize.X),
						Y:    uint32(winSize.Y),
					},
				},
			}
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.
}
