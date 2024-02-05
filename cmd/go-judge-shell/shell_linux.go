package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"github.com/criyle/go-judge/cmd/go-judge/stream"
)

func handleSizeChange(sendCh chan *stream.Request) {
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
			sendCh <- &stream.Request{
				Resize: &stream.ResizeRequest{
					Rows: int(winSize.Rows),
					Cols: int(winSize.Cols),
					X:    int(winSize.X),
					Y:    int(winSize.Y),
				},
			}
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.
}
