// Package signaling implements the WebSocket signaling adapter.
package signaling

import (
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 35 * time.Second
	pingPeriod     = 30 * time.Second
	maxMessageSize = 8192
	sendBufferSize = 8
)

// Client wraps a WebSocket connection with a dedicated write pump goroutine.
// All writes go through the send channel to avoid concurrent-write panics.
type Client struct {
	hub       *Hub
	conn      *websocket.Conn
	send      chan []byte
	roomID    string
	sessionID string // set after a successful "join" — used to route notifications
}

// writePump drains the send channel and writes to the WebSocket connection.
// Runs in its own goroutine. Exits when send is closed or a write fails.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait)) //nolint:errcheck // write will fail on next WriteMessage call if deadline is not set
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				slog.Error("websocket write", slog.Any("err", err))
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait)) //nolint:errcheck // write will fail on next WriteMessage call if deadline is not set
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump reads inbound messages and dispatches them to the hub.
// Exits on any read error, triggering unregistration and cleanup.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait)) //nolint:errcheck // read will fail at next ReadMessage if deadline is not set
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Error("websocket read", slog.Any("err", err))
			}
			return
		}
		c.hub.dispatch(c, data)
	}
}
