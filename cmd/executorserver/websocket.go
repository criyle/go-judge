package main

import (
	"context"
	"net/http"
	"time"

	"github.com/criyle/go-judge/cmd/executorserver/model"
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
	resultCh := make(chan model.Response, 128)
	// read request
	go func() {
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})

		ctx, cancel := context.WithCancel(context.TODO())
		defer cancel()

		for {
			req := new(model.Request)
			if err := conn.ReadJSON(req); err != nil {
				printLog("ws read:", err)
				return
			}
			r, err := model.ConvertRequest(req, *srcPrefix)
			if err != nil {
				printLog("convert", err)
				return
			}
			go func() {
				ret := <-work.Submit(ctx, r)
				execObserve(ret)
				resultCh <- model.ConvertResponse(ret)
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
