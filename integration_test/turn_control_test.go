//go:build integration

package integration_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type streamRequest struct {
	RequestID string `json:"requestId"`
	Cmd       []Cmd  `json:"cmd"`
}

type controlEvent struct {
	RequestID string `json:"requestId"`
	Index     int    `json:"index"`
	TurnID    uint64 `json:"turnId"`
	Type      string `json:"type"`
	Output    string `json:"output"`
}

func TestStreamTurnControlTwoAIs(t *testing.T) {
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:5050/stream", nil)
	if err != nil {
		t.Fatalf("dial stream: %v", err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	script := "import sys\nfor line in sys.stdin:\n print('MOVE '+line.strip(), flush=True)"
	cmd := Cmd{
		Args: []string{"python3", "-u", "-c", script},
		Env:  []string{"PATH=/usr/bin:/bin"},
		Files: []*CmdFile{
			{StreamIn: true},
			{StreamOut: true},
			{Name: "stderr", Max: 4096},
		},
		CPULimit: 5 * uint64(time.Second), MemoryLimit: 64 << 20, ProcLimit: 1,
	}
	execPayload, err := json.Marshal(streamRequest{RequestID: "turn-integration", Cmd: []Cmd{cmd, cmd}})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, append([]byte{1}, execPayload...)); err != nil {
		t.Fatal(err)
	}

	for index := 0; index < 2; index++ {
		controlPayload, err := json.Marshal(map[string]any{
			"index": index,
			"beginTurn": map[string]any{
				"turnId": index + 1, "moveCpuLimit": uint64(200 * time.Millisecond),
				"totalCpuLimit": uint64(2 * time.Second), "wallLimit": uint64(time.Second),
				"outputFd": 1, "delimiter": "\n", "maxOutput": 4096,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, append([]byte{5}, controlPayload...)); err != nil {
			t.Fatal(err)
		}
		input := []byte{3, byte(index << 4)}
		input = append(input, []byte("normal\n")...)
		if err := conn.WriteMessage(websocket.BinaryMessage, input); err != nil {
			t.Fatal(err)
		}

		for {
			_, frame, err := conn.ReadMessage()
			if err != nil {
				t.Fatal(err)
			}
			if len(frame) == 0 || frame[0] != 5 {
				continue
			}
			var event controlEvent
			if err := json.Unmarshal(frame[1:], &event); err != nil {
				t.Fatal(err)
			}
			if event.Type != "turnCompleted" || event.Index != index || event.Output != "MOVE normal\n" {
				t.Fatalf("unexpected event: %#v", event)
			}
			break
		}
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{4}); err != nil {
		t.Fatal(err)
	}
}
