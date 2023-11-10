package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/criyle/go-judge/pb"
	"golang.org/x/crypto/ssh/terminal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	srvAddr = flag.String("srvaddr", "localhost:5051", "GRPC server addr")
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

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		args = []string{"/bin/bash"}
	}

	token := os.Getenv("TOKEN")
	opts := []grpc.DialOption{grpc.WithInsecure()}
	if token != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(newTokenAuth(token)))
	}
	conn, err := grpc.Dial(*srvAddr, opts...)

	if err != nil {
		log.Fatalln("client", err)
	}
	client := pb.NewExecutorClient(conn)
	sc, err := client.ExecStream(context.TODO())
	if err != nil {
		log.Fatalln("ExecStream", err)
	}
	log.Println("Starts", args)
	r, err := run(sc, args)
	log.Println("ExecStream Finished", r, err)
}

func run(sc pb.Executor_ExecStreamClient, args []string) (*pb.Response, error) {
	req := &pb.Request{
		Cmd: []*pb.Request_CmdType{{
			Args: args,
			Env:  env,
			Files: []*pb.Request_File{
				{
					File: &pb.Request_File_StreamIn{
						StreamIn: &pb.Request_StreamInput{
							Name: "stdin",
						},
					},
				},
				{
					File: &pb.Request_File_StreamOut{
						StreamOut: &pb.Request_StreamOutput{
							Name: "stdout",
						},
					},
				},
				{
					File: &pb.Request_File_StreamOut{
						StreamOut: &pb.Request_StreamOutput{
							Name: "stderr",
						},
					},
				},
			},
			CpuTimeLimit:   uint64(cpuLimit),
			ClockTimeLimit: uint64(sessionLimit),
			MemoryLimit:    memoryLimit,
			ProcLimit:      procLimit,
			Tty:            true,
		}},
	}
	err := sc.Send(&pb.StreamRequest{
		Request: &pb.StreamRequest_ExecRequest{
			ExecRequest: req,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("ExecStream Send request %v", err)
	}

	// Set stdin in raw mode.
	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer func() { _ = terminal.Restore(int(os.Stdin.Fd()), oldState) }() // Best effort.

	// pump msg
	sendCh := make(chan *pb.StreamRequest, 64)
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
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if err == io.EOF {
				sendCh <- &pb.StreamRequest{
					Request: &pb.StreamRequest_ExecInput{
						ExecInput: &pb.StreamRequest_Input{
							Name:    "stdin",
							Content: []byte("\004"),
						},
					},
				}
				continue
			}
			if err != nil {
				log.Println("stdin", err)
				return
			}
			sendCh <- &pb.StreamRequest{
				Request: &pb.StreamRequest_ExecInput{
					ExecInput: &pb.StreamRequest_Input{
						Name:    "stdin",
						Content: buf[:n],
					},
				},
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	// pump ^C
	go func() {
		for range sigCh {
			sendCh <- &pb.StreamRequest{
				Request: &pb.StreamRequest_ExecInput{
					ExecInput: &pb.StreamRequest_Input{
						Name:    "stdin",
						Content: []byte("\003"),
					},
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
			return nil, fmt.Errorf("ExecStream recv %v", err)
		}
		switch sr := sr.Response.(type) {
		case *pb.StreamResponse_ExecOutput:
			switch sr.ExecOutput.Name {
			case "stdout":
				os.Stdout.Write(sr.ExecOutput.Content)
			case "stderr":
				os.Stderr.Write(sr.ExecOutput.Content)
			}
		case *pb.StreamResponse_ExecResponse:
			return sr.ExecResponse, nil
		}
	}
}

type tokenAuth struct {
	token string
}

func newTokenAuth(token string) credentials.PerRPCCredentials {
	return &tokenAuth{token: token}
}

// Return value is mapped to request headers.
func (t *tokenAuth) GetRequestMetadata(ctx context.Context, in ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + t.token,
	}, nil
}

func (*tokenAuth) RequireTransportSecurity() bool {
	return false
}
