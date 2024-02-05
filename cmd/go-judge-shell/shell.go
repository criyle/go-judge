package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/criyle/go-judge/cmd/go-judge/model"
	"github.com/criyle/go-judge/cmd/go-judge/stream"
	"golang.org/x/term"
)

var (
	srvAddr = flag.String("srv-addr", "localhost:5051", "GRPC server addr")
)

const (
	cpuLimit     = 20 * time.Second
	sessionLimit = 30 * time.Minute
	procLimit    = 50
	memoryLimit  = 256 << 20 // 256m
	pathEnv      = "PATH=/usr/local/bin:/usr/bin:/bin"
)

var env = []string{
	pathEnv,
	"HOME=/tmp",
	"TERM=" + os.Getenv("TERM"),
}

type Stream interface {
	Send(*stream.Request) error
	Recv() (*stream.Response, error)
}

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		args = []string{"/bin/bash"}
	}
	w := newGrpc(args, srvAddr)
	r, err := run(w, args)
	log.Printf("finished: %+v %v", r, err)
}

func stringPointer(s string) *string {
	return &s
}

func run(sc Stream, args []string) (*model.Response, error) {
	req := model.Request{
		Cmd: []model.Cmd{{
			Args: args,
			Env:  env,
			Files: []*model.CmdFile{
				{StreamIn: stringPointer("stdin")},
				{StreamOut: stringPointer("stdout")},
				{StreamOut: stringPointer("stderr")},
			},
			CPULimit:    uint64(cpuLimit.Nanoseconds()),
			ClockLimit:  uint64(sessionLimit.Nanoseconds()),
			MemoryLimit: memoryLimit,
			ProcLimit:   procLimit,
			TTY:         true,
		}},
	}
	err := sc.Send(&stream.Request{Request: &req})
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	// Set stdin in raw mode.
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }() // Best effort.

	// pump msg
	sendCh := make(chan *stream.Request, 64)
	defer close(sendCh)
	go func() {
		for r := range sendCh {
			err := sc.Send(r)
			if err != nil {
				log.Println("input", err)
				return
			}
		}
	}()

	// pump stdin
	forceQuit := false
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if err == io.EOF {
				sendCh <- &stream.Request{
					Input: &stream.InputRequest{
						Name:    "stdin",
						Content: []byte("\004"),
					},
				}
				continue
			}
			if n == 1 && buf[0] == 3 {
				if forceQuit {
					sendCh <- &stream.Request{
						Cancel: &struct{}{},
					}
				}
				forceQuit = true
			} else {
				forceQuit = false
			}

			if err != nil {
				log.Println("stdin", err)
				return
			}
			sendCh <- &stream.Request{
				Input: &stream.InputRequest{
					Name:    "stdin",
					Content: buf[:n],
				},
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	// pump ^C
	go func() {
		for range sigCh {
			sendCh <- &stream.Request{
				Input: &stream.InputRequest{
					Name:    "stdin",
					Content: []byte("\003"),
				},
			}
		}
	}()

	// pump resize
	handleSizeChange(sendCh)

	// pump stdout
	for {
		sr, err := sc.Recv()
		if err != nil {
			return nil, fmt.Errorf("recv: %w", err)
		}
		switch {
		case sr.Output != nil:
			switch sr.Output.Name {
			case "stdout":
				os.Stdout.Write(sr.Output.Content)
			case "stderr":
				os.Stderr.Write(sr.Output.Content)
			}
		case sr.Response != nil:
			return sr.Response, nil
		}
	}
}
