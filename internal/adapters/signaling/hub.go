package signaling

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/Sergiotsk/TalkGo/internal/ports/driving"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(_ *http.Request) bool { return true }, // permissive for MVP
}

// Hub manages connected WebSocket clients and dispatches signaling messages.
type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	handler    driving.SignalingHandler
	mu         sync.RWMutex
}

// NewHub creates a Hub that dispatches messages to the given SignalingHandler.
func NewHub(handler driving.SignalingHandler) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		handler:    handler,
	}
}

// Run processes client registration and unregistration events.
// Must be called in its own goroutine before any clients connect.
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = true
			h.mu.Unlock()

		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()
		}
	}
}

// ClientCount returns the number of currently registered clients.
// Exported for testing purposes.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ServeWS upgrades an HTTP connection to WebSocket and registers the client.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, roomID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade", slog.Any("err", err))
		return
	}
	c := &Client{
		hub:    h,
		conn:   conn,
		send:   make(chan []byte, sendBufferSize),
		roomID: roomID,
	}
	h.register <- c
	go c.writePump()
	go c.readPump()
}

// dispatch parses a raw JSON message and calls the SignalingHandler.
// Errors produce an "error" response sent back to the originating client.
func (h *Hub) dispatch(c *Client, data []byte) {
	var msg driving.SignalingMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		h.sendError(c, "invalid JSON")
		return
	}

	resp, err := h.handler.HandleSignaling(context.Background(), msg)
	if err != nil {
		h.sendError(c, err.Error())
		return
	}

	if resp.Type == "" {
		return // empty ACK — no response needed
	}

	b, err := json.Marshal(resp)
	if err != nil {
		slog.Error("marshalling signaling response", slog.Any("err", err))
		return
	}
	c.send <- b
}

func (h *Hub) sendError(c *Client, msg string) {
	resp := driving.SignalingMessage{Type: "error", Message: msg}
	if b, err := json.Marshal(resp); err == nil {
		c.send <- b
	}
}
