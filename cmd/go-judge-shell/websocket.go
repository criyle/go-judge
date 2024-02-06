package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/criyle/go-judge/cmd/go-judge/model"
	"github.com/criyle/go-judge/cmd/go-judge/stream"
	"github.com/gorilla/websocket"
)

var _ Stream = &websocketStream{}

type websocketStream struct {
	conn *websocket.Conn
}

func newWebsocket(args []string, wsURL string) Stream {
	header := make(http.Header)
	token := os.Getenv("TOKEN")
	if token != "" {
		header.Add("Authorization", "Bearer "+token)
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		log.Fatalln("ws connect: ", err)
	}
	log.Println("start", args)
	return &websocketStream{conn: conn}
}

// Recv implements Stream.
func (s *websocketStream) Recv() (*stream.Response, error) {
	_, r, err := s.conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	if len(r) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	resp := new(stream.Response)
	switch r[0] {
	case 1:
		resp.Response = new(model.Response)
		if err := json.Unmarshal(r[1:], resp.Response); err != nil {
			return nil, err
		}
	case 2:
		if len(r) < 2 {
			return nil, io.ErrUnexpectedEOF
		}
		resp.Output = new(stream.OutputResponse)
		resp.Output.Index = int(r[1]>>4) & 0xf
		resp.Output.Fd = int(r[1]) & 0xf
		resp.Output.Content = r[2:]
	default:
		return nil, fmt.Errorf("invalid type code: %d", r[0])
	}
	return resp, nil
}

// Send implements Stream.
func (s *websocketStream) Send(req *stream.Request) error {
	w, err := s.conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return err
	}
	defer w.Close()

	switch {
	case req.Request != nil:
		if _, err := w.Write([]byte{1}); err != nil {
			return err
		}
		if err := json.NewEncoder(w).Encode(req.Request); err != nil {
			return err
		}
	case req.Resize != nil:
		if _, err := w.Write([]byte{2}); err != nil {
			return err
		}
		if err := json.NewEncoder(w).Encode(req.Resize); err != nil {
			return err
		}
	case req.Input != nil:
		if _, err := w.Write([]byte{3, byte(req.Input.Index<<4 | req.Input.Fd)}); err != nil {
			return err
		}
		if _, err := w.Write(req.Input.Content); err != nil {
			return err
		}
	case req.Cancel != nil:
		if _, err := w.Write([]byte{4}); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid request")
	}
	return nil
}
