package envexec

import (
	"io"
	"os"
)

type pipeBuffer struct {
	W      *os.File
	Buffer *os.File
	Done   <-chan struct{}
	Limit  Size
}

type pipeCollector struct {
	done    <-chan struct{}
	buffer  *os.File
	limit   Size
	name    string
	storage bool
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

func newPipeBuffer(limit Size, newFile NewStoreFile) (*pipeBuffer, error) {
	buffer, err := newFile()
	if err != nil {
		return nil, err
	}
	done, w, err := newPipe(buffer, limit+1)
	if err != nil {
		buffer.Close()
		return nil, err
	}
	return &pipeBuffer{
		W:      w,
		Buffer: buffer,
		Done:   done,
		Limit:  limit,
	}, nil
}
