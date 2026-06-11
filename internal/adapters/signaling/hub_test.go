package signaling_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Sergiotsk/TalkGo/internal/adapters/signaling"
	"github.com/Sergiotsk/TalkGo/internal/ports/driving"
)

// mockHandler is a simple SignalingHandler for testing.
type mockHandler struct {
	response driving.SignalingMessage
	err      error
}

func (m *mockHandler) HandleSignaling(_ context.Context, _ driving.SignalingMessage) (driving.SignalingMessage, error) { //nolint:gocritic // value receiver matches interface; SignalingMessage is a DTO
	return m.response, m.err
}

func (m *mockHandler) OnDisconnect(_ context.Context, _ string) error { return nil }

func dialHub(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	u := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

func TestHub_RegisterClient(t *testing.T) {
	hub := signaling.NewHub(&mockHandler{response: driving.SignalingMessage{}})
	go hub.Run()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r, "room-1")
	}))
	defer srv.Close()

	conn := dialHub(t, srv)
	defer conn.Close()

	// Allow time for registration goroutine
	time.Sleep(50 * time.Millisecond)

	if hub.ClientCount() != 1 {
		t.Errorf("expected 1 registered client, got %d", hub.ClientCount())
	}
}

func TestHub_UnregisterOnDisconnect(t *testing.T) {
	hub := signaling.NewHub(&mockHandler{response: driving.SignalingMessage{}})
	go hub.Run()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r, "room-1")
	}))
	defer srv.Close()

	conn := dialHub(t, srv)
	time.Sleep(50 * time.Millisecond)

	if hub.ClientCount() != 1 {
		t.Fatalf("expected 1 client before disconnect, got %d", hub.ClientCount())
	}

	conn.Close()
	time.Sleep(100 * time.Millisecond)

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients after disconnect, got %d", hub.ClientCount())
	}
}

func TestHub_DispatchAndReceiveResponse(t *testing.T) {
	expected := driving.SignalingMessage{Type: "joined", SessionID: "sess-abc", RoomID: "room-1"}
	hub := signaling.NewHub(&mockHandler{response: expected})
	go hub.Run()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r, "room-1")
	}))
	defer srv.Close()

	conn := dialHub(t, srv)
	defer conn.Close()
	time.Sleep(50 * time.Millisecond)

	// Send a join message
	msg := driving.SignalingMessage{Type: "join", RoomID: "room-1", UserID: "user-1"}
	b, _ := json.Marshal(msg)
	if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("write: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var resp driving.SignalingMessage
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Type != "joined" {
		t.Errorf("got type %q, want %q", resp.Type, "joined")
	}
	if resp.SessionID != "sess-abc" {
		t.Errorf("got session_id %q, want %q", resp.SessionID, "sess-abc")
	}
}

func TestHub_InvalidJSONReturnsError(t *testing.T) {
	hub := signaling.NewHub(&mockHandler{response: driving.SignalingMessage{}})
	go hub.Run()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r, "room-1")
	}))
	defer srv.Close()

	conn := dialHub(t, srv)
	defer conn.Close()
	time.Sleep(50 * time.Millisecond)

	if err := conn.WriteMessage(websocket.TextMessage, []byte("{not valid json")); err != nil {
		t.Fatalf("write: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var resp driving.SignalingMessage
	_ = json.Unmarshal(data, &resp)
	if resp.Type != "error" {
		t.Errorf("expected error response for invalid JSON, got type=%q", resp.Type)
	}
}
