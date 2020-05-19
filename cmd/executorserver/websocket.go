package main

import (
	"net/http"
	"time"

	"github.com/criyle/go-judge/worker"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 50 * time.Second
)

func handleWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.Error(err)
		c.AbortWithStatusJSON(http.StatusBadRequest, err.Error())
		return
	}
	resultCh := make(chan worker.Result, 128)
	// read request
	go func() {
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})
		for {
			req := new(worker.Request)
			if err := conn.ReadJSON(req); err != nil {
				printLog("ws read:", err)
				return
			}
			go func() {
				resultCh <- <-work.Submit(req)
			}()
		}
	}()

	// write result
	go func() {
		defer conn.Close()
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case r := <-resultCh:
				if r.Error != nil {
					r.ErrorMsg = r.Error.Error()
				}
				conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteJSON(r); err != nil {
					printLog("ws write:", err)
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()
}
