package signaling

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
	"github.com/Sergiotsk/TalkGo/internal/ports/driving"
)

// Compile-time check: Hub must satisfy driven.EventNotifier.
var _ driven.EventNotifier = (*Hub)(nil)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(_ *http.Request) bool { return true }, // permissive for MVP
}

// Hub manages connected WebSocket clients and dispatches signaling messages.
type Hub struct {
	clients        map[*Client]bool
	register       chan *Client
	unregister     chan *Client
	handler        driving.SignalingHandler
	sessionClients map[string]*Client   // sessionID → Client
	roomClients    map[string][]*Client // roomID → []Client (for peer-left routing)
	mu             sync.RWMutex
}

// NewHub creates a Hub that dispatches messages to the given SignalingHandler.
// handler may be nil if SetHandler will be called before any clients connect.
func NewHub(handler driving.SignalingHandler) *Hub {
	return &Hub{
		clients:        make(map[*Client]bool),
		register:       make(chan *Client),
		unregister:     make(chan *Client),
		handler:        handler,
		sessionClients: make(map[string]*Client),
		roomClients:    make(map[string][]*Client),
	}
}

// SetHandler sets the SignalingHandler. Must be called before any clients connect
// when the handler cannot be provided at construction time (e.g., circular wiring).
func (h *Hub) SetHandler(handler driving.SignalingHandler) {
	h.mu.Lock()
	h.handler = handler
	h.mu.Unlock()
}

// Run processes client registration and unregistration events.
// Must be called in its own goroutine before any clients connect.
// Deprecated: prefer RunCtx which supports graceful shutdown via context.
func (h *Hub) Run() {
	h.RunCtx(context.Background())
}

// RunCtx processes client registration and unregistration events.
// Exits when ctx is cancelled, enabling graceful shutdown.
// Must be called in its own goroutine before any clients connect.
func (h *Hub) RunCtx(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = true
			if c.roomID != "" {
				h.roomClients[c.roomID] = append(h.roomClients[c.roomID], c)
			}
			h.mu.Unlock()

		case c := <-h.unregister:
			var sessionID string
			var roomID string

			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			if c.sessionID != "" {
				sessionID = c.sessionID
				delete(h.sessionClients, c.sessionID)
			}
			roomID = c.roomID
			// Remove client from roomClients slice.
			if roomID != "" {
				peers := h.roomClients[roomID]
				updated := peers[:0]
				for _, p := range peers {
					if p != c {
						updated = append(updated, p)
					}
				}
				if len(updated) == 0 {
					delete(h.roomClients, roomID)
				} else {
					h.roomClients[roomID] = updated
				}
			}
			// Collect peer clients in the same room (for peer-left notification).
			var peerClients []*Client
			if sessionID != "" && roomID != "" {
				for _, peer := range h.roomClients[roomID] {
					if peer != c {
						peerClients = append(peerClients, peer)
					}
				}
			}
			h.mu.Unlock()

			// Send peer-left to remaining room peers — OUTSIDE the mutex.
			if sessionID != "" && len(peerClients) > 0 {
				peerLeftMsg := map[string]string{
					"type":       "peer-left",
					"session_id": sessionID,
				}
				if data, err := json.Marshal(peerLeftMsg); err == nil {
					for _, peer := range peerClients {
						select {
						case peer.send <- data:
						default:
							// peer buffer full — drop
						}
					}
				}
			}

			// Call OnDisconnect on the handler — OUTSIDE the mutex (prevents deadlock
			// if OnDisconnect triggers NotifySession which acquires the same mutex).
			if sessionID != "" {
				h.mu.RLock()
				handler := h.handler
				h.mu.RUnlock()
				if handler != nil {
					if err := handler.OnDisconnect(context.Background(), sessionID); err != nil {
						slog.Error("on_disconnect_error",
							"component", "hub",
							"session_id", sessionID,
							slog.Any("err", err))
					}
				}
			}
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
		slog.Error("ws_upgrade_failed", "component", "hub", slog.Any("err", err))
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
// When the response type is "joined", the client is registered by sessionID
// so that NotifySession can reach it later.
func (h *Hub) dispatch(c *Client, data []byte) {
	var msg driving.SignalingMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		h.sendError(c, "invalid JSON")
		return
	}

	h.mu.RLock()
	handler := h.handler
	h.mu.RUnlock()

	resp, err := handler.HandleSignaling(context.Background(), msg)
	if err != nil {
		h.sendError(c, err.Error())
		return
	}

	// After a successful join, bind sessionID → client for future notifications.
	if resp.Type == "joined" && resp.SessionID != "" {
		h.mu.Lock()
		c.sessionID = resp.SessionID
		h.sessionClients[resp.SessionID] = c
		h.mu.Unlock()
	}

	if resp.Type == "" {
		return // empty ACK — no response needed
	}

	b, err := json.Marshal(resp)
	if err != nil {
		slog.Error("signal_response_marshal_error", "component", "hub", slog.Any("err", err))
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

// NotifySession sends a message to the client associated with sessionID.
// If the session is not connected or the client send buffer is full, the message is silently dropped.
func (h *Hub) NotifySession(sessionID, msgType string, fields map[string]string) {
	h.mu.RLock()
	client, ok := h.sessionClients[sessionID]
	h.mu.RUnlock()
	if !ok {
		slog.Warn("notify_session_not_found", "component", "hub", "session_id", sessionID, "type", msgType)
		return
	}
	slog.Info("notify_session_sent", "component", "hub", "session_id", sessionID, "type", msgType)

	msg := make(map[string]string, len(fields)+1)
	msg["type"] = msgType
	for k, v := range fields {
		msg[k] = v
	}

	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("notify_session_marshal_error", "component", "hub", slog.Any("err", err))
		return
	}

	select {
	case client.send <- data:
	default:
		// client buffer full — drop to avoid blocking
	}
}
