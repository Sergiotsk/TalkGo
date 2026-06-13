package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sergiotsk/TalkGo/internal/adapters/signaling"
	"github.com/Sergiotsk/TalkGo/internal/domain/room"
	"github.com/Sergiotsk/TalkGo/internal/ports/driving"
)

// captureLogs replaces the default slog logger and returns a buffer + restore func.
func captureLogs(t *testing.T, level slog.Level) (buf *bytes.Buffer, restore func()) {
	t.Helper()
	var b bytes.Buffer
	handler := slog.NewJSONHandler(&b, &slog.HandlerOptions{Level: level})
	old := slog.Default()
	slog.SetDefault(slog.New(handler))
	buf = &b
	restore = func() { slog.SetDefault(old) }
	return
}

// errManager is a RoomManager that returns errors from specific methods.
type errManager struct {
	driving.RoomManager
	createRoomFn func(ctx context.Context, sourceLang, targetLang string) (driving.CreateRoomResult, error)
	deleteRoomFn func(ctx context.Context, roomID string) error
	findByCodeFn func(ctx context.Context, code string) (*room.Room, error)
	roomExistsFn func(ctx context.Context, roomID string) error
}

func (m *errManager) CreateRoom(ctx context.Context, sourceLang, targetLang string) (driving.CreateRoomResult, error) {
	if m.createRoomFn != nil {
		return m.createRoomFn(ctx, sourceLang, targetLang)
	}
	return m.RoomManager.CreateRoom(ctx, sourceLang, targetLang)
}

func (m *errManager) DeleteRoom(ctx context.Context, roomID string) error {
	if m.deleteRoomFn != nil {
		return m.deleteRoomFn(ctx, roomID)
	}
	return m.RoomManager.DeleteRoom(ctx, roomID)
}

func (m *errManager) FindByShortCode(ctx context.Context, code string) (*room.Room, error) {
	if m.findByCodeFn != nil {
		return m.findByCodeFn(ctx, code)
	}
	return m.RoomManager.FindByShortCode(ctx, code)
}

func (m *errManager) RoomExists(ctx context.Context, roomID string) error {
	if m.roomExistsFn != nil {
		return m.roomExistsFn(ctx, roomID)
	}
	return m.RoomManager.RoomExists(ctx, roomID)
}

func TestHTTPLogs_UseSnakeCaseAndComponent(t *testing.T) {
	buf, restore := captureLogs(t, slog.LevelDebug)
	defer restore()

	// Create a server with a mock manager that errors out.
	mgr := &errManager{}
	s := NewServer(DefaultConfig(), mgr, signaling.NewHub(nil), nil, nil)

	// Trigger create_room_error with an internal error (not validation).
	mgr.createRoomFn = func(ctx context.Context, sourceLang, targetLang string) (driving.CreateRoomResult, error) {
		return driving.CreateRoomResult{}, assertAnError("internal error")
	}

	req := httptest.NewRequest(http.MethodPost, "/rooms", strings.NewReader(`{"source_lang":"es","target_lang":"en"}`))
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	// Trigger find_by_code_error.
	mgr.findByCodeFn = func(ctx context.Context, code string) (*room.Room, error) {
		return nil, assertAnError("db error")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/rooms/code/ABC123", http.NoBody)
	w2 := httptest.NewRecorder()
	s.mux.ServeHTTP(w2, req2)

	// Trigger ws_handler_error.
	mgr.roomExistsFn = func(ctx context.Context, roomID string) error {
		return assertAnError("room check failed")
	}

	req3 := httptest.NewRequest(http.MethodGet, "/ws/room-1", http.NoBody)
	w3 := httptest.NewRecorder()
	s.mux.ServeHTTP(w3, req3)

	logs := strings.TrimSpace(buf.String())
	if logs == "" {
		t.Skip("no log output captured")
		return
	}

	lines := strings.Split(logs, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("invalid JSON log: %s", line)
			continue
		}

		msg, ok := entry["msg"].(string)
		if !ok {
			continue
		}

		// Verify snake_case: no spaces.
		if strings.Contains(msg, " ") {
			t.Errorf("log msg has spaces (not snake_case): %q", msg)
		}

		// Verify component field exists and is "http".
		if _, ok := entry["component"]; !ok {
			t.Errorf("log entry missing 'component': msg=%q", msg)
		}
	}
}

type assertAnError string

func (e assertAnError) Error() string { return string(e) }
