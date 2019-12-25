package syzojclient

import (
	"log"
	"net/http"
	"reflect"

	"github.com/criyle/go-judge/client"
	"github.com/criyle/go-judge/types"

	engineio "github.com/googollee/go-engine.io"
	"github.com/googollee/go-engine.io/transport"
	"github.com/googollee/go-engine.io/transport/polling"
	"github.com/googollee/go-engine.io/transport/websocket"
	"github.com/googollee/go-socket.io/parser"
	"github.com/ugorji/go/codec"
)

const (
	buffSize  = 64
	namespace = "/judge"
)

var dialar = &engineio.Dialer{
	Transports: []transport.Transport{websocket.Default, polling.Default},
}

type ack struct {
	id uint64
}

// Client is syzoj judge client
type Client struct {
	Done chan struct{}

	token string

	socket   engineio.Conn
	tasks    chan client.Task
	progress chan *types.JudgeProgress
	finish   chan *types.JudgeResult
	request  chan struct{}
	ack      chan ack

	encoder *parser.Encoder
	decoder *parser.Decoder

	errCh chan error
}

// NewClient connect to socket.io endpoint
func NewClient(url, token string) (*Client, chan error, error) {
	socket, err := dialar.Dial(url, http.Header{})
	if err != nil {
		return nil, nil, err
	}

	c := &Client{
		Done:     make(chan struct{}),
		token:    token,
		socket:   socket,
		tasks:    make(chan client.Task, buffSize),
		progress: make(chan *types.JudgeProgress, buffSize),
		finish:   make(chan *types.JudgeResult, buffSize),
		request:  make(chan struct{}, 1),
		ack:      make(chan ack, 1),
		encoder:  parser.NewEncoder(socket),
		decoder:  parser.NewDecoder(socket),
		errCh:    make(chan error),
	}

	go c.readLoop()
	go c.writeLoop()

	return c, c.errCh, nil
}

// C c
func (c *Client) C() <-chan client.Task {
	return c.tasks
}

func (c *Client) readLoop() (err error) {
	// handle error
	defer func() {
		if err != nil {
			select {
			case c.errCh <- err:
			default:
			}
		}
	}()

	var (
		event  string
		header parser.Header
	)
	taskType := []reflect.Type{reflect.TypeOf((*parser.Buffer)(nil))}

	for {
		if err := c.decoder.DecodeHeader(&header, &event); err != nil {
			return err
		}
		switch header.Type {
		case parser.Event:
			switch event {
			case "onTask":
				// receive binary message
				args, err := c.decoder.DecodeArgs(taskType)
				if err != nil {
					return err
				}
				buf := args[0].Interface().(*parser.Buffer)

				// decode msgPack
				var task judgeTask
				if err := codec.NewDecoderBytes(buf.Data, &codec.MsgpackHandle{}).Decode(&task); err != nil {
					return err
				}
				c.tasks <- newTask(c, &task, header.ID)
			}

		case parser.Connect:
			// if connected to namespace, emit waitForTask
			if header.Namespace == namespace {
				c.request <- struct{}{}
			}

			c.decoder.DiscardLast()

		default:
			c.decoder.DiscardLast()
		}
	}
}

func (c *Client) writeLoop() (err error) {
	// handle error
	defer func() {
		if err != nil {
			select {
			case c.errCh <- err:
			default:
			}
		}
	}()

	// connect to judge
	if err := c.encoder.Encode(parser.Header{
		Type:      parser.Connect,
		Namespace: namespace,
	}, nil); err != nil {
		return err
	}

	for {
		select {
		case <-c.Done:
			return

		case <-c.request:
			if err := c.encoder.Encode(parser.Header{
				Type:      parser.Event,
				Namespace: namespace,
				NeedAck:   true,
			}, []interface{}{"waitForTask", c.token}); err != nil {
				return err
			}

		case a := <-c.ack:
			if err := c.encoder.Encode(parser.Header{
				Type:      parser.Ack,
				Namespace: namespace,
				ID:        a.id,
				NeedAck:   true,
			}, nil); err != nil {
				return err
			}
		}
	}
}

type judgeTask struct {
	Content   judgeTaskContent `json:"content"`
	ExtraData string           `json:"extraData"`
}

type judgeTaskContent struct {
	TaskID   string         `json:"taskId"`
	TestData string         `json:"testData"`
	Type     int            `json:"type"`
	Priority int            `json:"priority"`
	Param    judgeParameter `json:"param"`
}

type judgeParameter struct {
	Language    string `json:"language"`
	Code        string `json:"code"`
	TimeLimit   uint64 `json:"timeLimit"`
	MemoryLimit uint64 `json:"memoryLimit"`
	// standard
	FileIOInput  *string `json:"fileIOInput"`
	FileIOOutput *string `json:"fileIOOutput"`
	// interaction
}

type judgeProgress struct {
	TaskID   string              `json:"taskId"`
	Type     int                 `json:"type"`
	Progress judgeResultProgress `json:"progress"`
}

type judgeResultProgress struct {
	Subtasks []subtaskResult `json:"subtasks"`
}

type subtaskResult struct {
	Time   uint64 `json:"time"`
	Memory uint64 `json:"memory"`
}

func newTask(c *Client, msg *judgeTask, ackID uint64) client.Task {
	task := &types.JudgeTask{
		//Type:        msg.Content.Type,
		Language:    msg.Content.Param.Language,
		Code:        msg.Content.Param.Code,
		TileLimit:   msg.Content.Param.TimeLimit,
		MemoryLimit: msg.Content.Param.MemoryLimit << 10,
	}

	log.Println(msg)

	return &Task{
		client: c,
		task:   task,
		ackID:  ackID,
	}
}
