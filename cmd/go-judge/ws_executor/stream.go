package wsexecutor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/criyle/go-judge/cmd/go-judge/model"
	"github.com/criyle/go-judge/cmd/go-judge/stream"
	"github.com/gorilla/websocket"
)

var _ stream.Stream = &streamWrapper{}

const maxPackedStreamField = 15

var errStreamFieldTooLarge = errors.New("stream index or fd exceeds websocket wire limit")

type streamWrapper struct {
	ctx    context.Context
	conn   *websocket.Conn
	sendCh chan stream.Response
}

func (w *streamWrapper) sendLoop() {
	conn := w.conn
	defer conn.Close()

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-w.ctx.Done():
			return
		case r := <-w.sendCh:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			switch {
			case r.Response != nil:
				w, err := conn.NextWriter(websocket.BinaryMessage)
				if err != nil {
					return
				}
				if _, err := w.Write([]byte{1}); err != nil {
					return
				}
				if err := json.NewEncoder(w).Encode(r.Response); err != nil {
					return
				}
				if err := w.Close(); err != nil {
					return
				}
				conn.Close()
				return
			case r.Output != nil:
				if r.Output.Index > maxPackedStreamField || r.Output.Fd > maxPackedStreamField {
					return
				}
				w, err := conn.NextWriter(websocket.BinaryMessage)
				if err != nil {
					return
				}
				if _, err := w.Write([]byte{2, byte(r.Output.Index<<4 | r.Output.Fd)}); err != nil {
					return
				}
				if _, err := w.Write(r.Output.Content); err != nil {
					return
				}
				if err := w.Close(); err != nil {
					return
				}
			}
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (w *streamWrapper) Send(resp stream.Response) error {
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case w.sendCh <- resp:
		return nil
	}
}

func (w *streamWrapper) Recv() (*stream.Request, error) {
	conn := w.conn
	_, buf, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	if len(buf) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	var req stream.Request
	switch buf[0] {
	case 1:
		req.Request = new(model.Request)
		if err := json.Unmarshal(buf[1:], req.Request); err != nil {
			return nil, err
		}
		if err := validateStreamRequestLimits(req.Request); err != nil {
			return nil, err
		}
	case 2:
		req.Resize = new(stream.ResizeRequest)
		if err := json.Unmarshal(buf[1:], req.Resize); err != nil {
			return nil, err
		}
	case 3:
		if len(buf) < 2 {
			return nil, io.ErrUnexpectedEOF
		}
		req.Input = new(stream.InputRequest)
		req.Input.Index = int(buf[1]>>4) & 0xf
		req.Input.Fd = int(buf[1]) & 0xf
		req.Input.Content = buf[2:]
	case 4:
		req.Cancel = new(struct{})
	default:
		return nil, fmt.Errorf("invalid type code: %d", buf[0])
	}
	return &req, nil
}

func validateStreamRequestLimits(req *model.Request) error {
	for i, c := range req.Cmd {
		if i > maxPackedStreamField {
			for _, f := range c.Files {
				if f != nil && (f.StreamIn || f.StreamOut) {
					return fmt.Errorf("%w: cmd index %d", errStreamFieldTooLarge, i)
				}
			}
		}
		for j, f := range c.Files {
			if f == nil || (!f.StreamIn && !f.StreamOut) {
				continue
			}
			if j > maxPackedStreamField {
				return fmt.Errorf("%w: fd %d", errStreamFieldTooLarge, j)
			}
		}
	}
	return nil
}
