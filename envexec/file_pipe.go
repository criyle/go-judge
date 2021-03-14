package envexec

import (
	"bytes"
	"io"
	"os"
)

type pipeBuffer struct {
	W      *os.File
	Buffer *bytes.Buffer
	Done   <-chan struct{}
	Limit  Size
}

type pipeCollector struct {
	done   <-chan struct{}
	buffer *bytes.Buffer
	limit  Size
	name   string
}

func newPipe(writer io.Writer, limit Size) (<-chan struct{}, *os.File, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	done := make(chan struct{})
	go func() {
		io.CopyN(writer, r, int64(limit))
		close(done)
		// ensure no blocking / SIGPIPE on the other end
		io.Copy(io.Discard, r)
		r.Close()
	}()
	return done, w, nil
}

func newPipeBuffer(limit Size) (*pipeBuffer, error) {
	buffer := new(bytes.Buffer)
	done, w, err := newPipe(buffer, limit+1)
	if err != nil {
		return nil, err
	}
	return &pipeBuffer{
		W:      w,
		Buffer: buffer,
		Done:   done,
		Limit:  limit,
	}, nil
}
