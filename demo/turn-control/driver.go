//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

type event struct {
	Index  int    `json:"index"`
	TurnID uint64 `json:"turnId"`
	Type   string `json:"type"`
	Output string `json:"output"`
}

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("usage: %s /absolute/path/to/ai", os.Args[0])
	}
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:5050/stream", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	cmd := map[string]any{
		"args": []string{os.Args[1]},
		"files": []any{
			map[string]bool{"streamIn": true},
			map[string]bool{"streamOut": true},
			map[string]any{"name": "stderr", "max": 4096},
		},
		"cpuLimit": uint64(10 * time.Second), "clockLimit": uint64(30 * time.Second),
		"memoryLimit": 64 << 20, "procLimit": 4,
	}
	exec := map[string]any{"requestId": "turn-demo", "cmd": []any{cmd, cmd}}
	writeJSON(conn, 1, exec)

	runTurn(conn, 0, 1, "normal\n", 200*time.Millisecond, 2*time.Second)
	runTurn(conn, 1, 2, "normal\n", 200*time.Millisecond, 2*time.Second)
	runTurn(conn, 0, 3, "total-step\n", 200*time.Millisecond, 150*time.Millisecond)
	runTurn(conn, 0, 4, "total-step\n", 200*time.Millisecond, 150*time.Millisecond)
}

func runTurn(conn *websocket.Conn, index int, turnID uint64, input string, moveLimit, totalLimit time.Duration) {
	writeJSON(conn, 5, map[string]any{
		"index": index,
		"beginTurn": map[string]any{
			"turnId": turnID, "moveCpuLimit": uint64(moveLimit),
			"totalCpuLimit": uint64(totalLimit), "wallLimit": uint64(2 * time.Second),
			"outputFd": 1, "delimiter": "\n", "maxOutput": 4096,
		},
	})
	frame := append([]byte{3, byte(index << 4)}, input...)
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		log.Fatal(err)
	}
	for {
		_, frame, err := conn.ReadMessage()
		if err != nil {
			log.Fatal(err)
		}
		if len(frame) == 0 || frame[0] != 5 {
			continue
		}
		var e event
		if err := json.Unmarshal(frame[1:], &e); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("AI %d turn %d: %s %q\n", e.Index, e.TurnID, e.Type, e.Output)
		return
	}
}

func writeJSON(conn *websocket.Conn, typ byte, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		log.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, append([]byte{typ}, payload...)); err != nil {
		log.Fatal(err)
	}
}
