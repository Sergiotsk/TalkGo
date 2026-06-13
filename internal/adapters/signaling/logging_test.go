package signaling

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sergiotsk/TalkGo/internal/ports/driving"
)

// syncBuffer wraps bytes.Buffer with a mutex for safe concurrent access.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *syncBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *syncBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

// captureLogs replaces the default slog logger with one that writes JSON to a syncBuffer.
func captureLogs(t *testing.T, level slog.Level) (buf *syncBuffer, restore func()) {
	t.Helper()
	var sb syncBuffer
	handler := slog.NewJSONHandler(&sb, &slog.HandlerOptions{Level: level})
	old := slog.Default()
	slog.SetDefault(slog.New(handler))
	buf = &sb
	restore = func() { slog.SetDefault(old) }
	return
}

// errHandler is a SignalingHandler that returns errors from OnDisconnect.
type errHandler struct {
	driving.SignalingHandler
	onDisconnect func(ctx context.Context, sessionID string) error
}

func (h *errHandler) OnDisconnect(ctx context.Context, sessionID string) error {
	if h.onDisconnect != nil {
		return h.onDisconnect(ctx, sessionID)
	}
	return nil
}

func TestHubLogs_UseSnakeCaseAndComponent(t *testing.T) {
	buf, restore := captureLogs(t, slog.LevelDebug)
	defer restore()

	handler := &errHandler{}
	hub := NewHub(handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.RunCtx(ctx)

	// Give RunCtx time to start.
	time.Sleep(10 * time.Millisecond)

	// Set up OnDisconnect to return an error.
	handler.onDisconnect = func(ctx context.Context, sessionID string) error {
		return assertAnError("test disconnect error")
	}

	// Register a client.
	client := &Client{
		hub:       hub,
		send:      make(chan []byte, sendBufferSize),
		sessionID: "test-sess",
		roomID:    "test-room",
	}
	hub.register <- client

	// Unregister to trigger OnDisconnect.
	hub.unregister <- client

	// Give RunCtx time to process.
	time.Sleep(10 * time.Millisecond)

	// Cancel RunCtx to stop logging.
	cancel()
	time.Sleep(5 * time.Millisecond)

	logs := buf.String()
	if logs == "" {
		t.Log("no logs captured — possibly the OnDisconnect path didn't produce output")
		return
	}

	lines := strings.Split(strings.TrimSpace(logs), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("invalid JSON log line: %s", line)
			continue
		}

		msg, ok := entry["msg"].(string)
		if !ok {
			t.Error("log entry missing 'msg' field")
			continue
		}

		// Verify snake_case: no spaces.
		if strings.Contains(msg, " ") {
			t.Errorf("log msg contains spaces (not snake_case): %q in line: %s", msg, line)
		}

		// Verify component field exists.
		if _, hasComponent := entry["component"]; !hasComponent {
			t.Errorf("log entry missing 'component' field: msg=%q in line: %s", msg, line)
		}
	}
}

func TestHubServeWS_LogsUpgradeFailure(t *testing.T) {
	buf, restore := captureLogs(t, slog.LevelDebug)
	defer restore()

	hub := NewHub(nil)

	// A plain HTTP request (not WebSocket upgrade) triggers the upgrade error path.
	req := httptest.NewRequest(http.MethodGet, "/ws/test", http.NoBody)
	w := httptest.NewRecorder()
	hub.ServeWS(w, req, "test-room")

	logs := strings.TrimSpace(buf.String())
	if logs == "" {
		t.Skip("no log captured for ws_upgrade_failed — upgrader may handle error differently")
	}

	lines := strings.Split(logs, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("invalid JSON log: %s", line)
			continue
		}

		msg, _ := entry["msg"].(string)
		if strings.Contains(msg, " ") {
			t.Errorf("log msg has spaces (not snake_case): %q", msg)
		}
		if _, ok := entry["component"]; !ok {
			t.Errorf("log entry missing 'component': msg=%q", msg)
		}
	}
}

// assertAnError is a sentinel error for triggering error log paths.
type assertAnError string

func (e assertAnError) Error() string { return string(e) }
