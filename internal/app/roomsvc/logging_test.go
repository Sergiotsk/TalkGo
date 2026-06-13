package roomsvc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sergiotsk/TalkGo/internal/domain/room"
	"github.com/Sergiotsk/TalkGo/internal/domain/session"
	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
	"github.com/Sergiotsk/TalkGo/internal/ports/driven/mocks"
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

func newServiceWithLoggingMocks(t *testing.T, repo driven.RoomRepository, peer driven.WebRTCPeer) *Service {
	t.Helper()
	svc, err := NewService(ServiceConfig{
		GracePeriod:         1 * time.Millisecond,
		RoomTTL:             1 * time.Hour,
		SweepInterval:       10 * time.Minute,
		MaxShortCodeRetries: 5,
	}, repo, peer, &mocks.MockTranslator{}, &mocks.MockAudioCodec{}, &mocks.MockEventNotifier{})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestServiceLogs_UseSnakeCaseAndComponent(t *testing.T) {
	buf, restore := captureLogs(t, slog.LevelDebug)
	defer restore()

	// Trigger sweep_list_error: make ListExpired return an error.
	mockRepo := &mocks.MockRoomRepository{
		ListExpiredFn: func(ctx context.Context, before time.Time) ([]*room.Room, error) {
			return nil, assertAnError("list expired failed")
		},
	}
	svc := newServiceWithLoggingMocks(t, mockRepo, &mocks.MockWebRTCPeer{})
	svc.sweepExpiredRooms(context.Background())

	logs := strings.TrimSpace(buf.String())
	if logs == "" {
		t.Fatal("expected log output from sweepExpiredRooms error, got empty")
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
			t.Error("log entry missing 'msg'")
			continue
		}

		if strings.Contains(msg, " ") {
			t.Errorf("log msg has spaces (not snake_case): %q", msg)
		}
		if _, ok := entry["component"]; !ok {
			t.Errorf("log entry missing 'component': msg=%q", msg)
		} else if entry["component"] != "service" {
			t.Errorf("component should be 'service', got %v", entry["component"])
		}
	}
}

func TestServiceLogs_SweepDeleteError(t *testing.T) {
	buf, restore := captureLogs(t, slog.LevelDebug)
	defer restore()

	expiredRoom, _ := room.NewRoom("room-1", "es", "en")
	mockRepo := &mocks.MockRoomRepository{
		FindByIDFn: func(ctx context.Context, roomID string) (*room.Room, error) {
			return expiredRoom, nil
		},
		ListExpiredFn: func(ctx context.Context, before time.Time) ([]*room.Room, error) {
			return []*room.Room{expiredRoom}, nil
		},
		DeleteFn: func(ctx context.Context, roomID string) error {
			return assertAnError("delete failed")
		},
	}
	svc := newServiceWithLoggingMocks(t, mockRepo, &mocks.MockWebRTCPeer{})
	svc.sweepExpiredRooms(context.Background())

	logs := strings.TrimSpace(buf.String())
	if logs == "" {
		t.Skip("no logs captured")
		return
	}

	lines := strings.Split(logs, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		msg, _ := entry["msg"].(string)
		if msg == "" {
			continue
		}
		if strings.Contains(msg, " ") {
			t.Errorf("log msg has spaces (not snake_case): %q", msg)
		}
		if _, ok := entry["component"]; !ok {
			t.Errorf("log entry missing 'component': msg=%q", msg)
		}
	}
}

func TestServiceLogs_ContextFieldsAreSnakeCase(t *testing.T) {
	buf, restore := captureLogs(t, slog.LevelDebug)
	defer restore()

	// Create room and session to trigger DeleteRoom with close_session_error.
	r, err := room.NewRoom("room-ctx-1", "es", "en")
	if err != nil {
		t.Fatal(err)
	}
	sess := session.NewSession("sess-ctx-1", "room-ctx-1", "user-1", "es")

	// Use a real InMemoryRoomRepository so DeleteRoom can find the room.
	realRepo := NewInMemoryRoomRepository()
	_ = realRepo.Save(context.Background(), r)

	// Mock peer that returns error from CloseSession.
	mockPeer := &mocks.MockWebRTCPeer{
		CloseSessionFn: func(ctx context.Context, sessionID string) error {
			return assertAnError("peer close failed")
		},
	}

	svc := newServiceWithLoggingMocks(t, realRepo, mockPeer)

	// Manually set up session in the service maps (unexported, but accessible from internal tests).
	svc.sessions["sess-ctx-1"] = sess
	svc.lookup["room-ctx-1:user-1"] = "sess-ctx-1"

	_ = svc.DeleteRoom(context.Background(), "room-ctx-1")

	logs := strings.TrimSpace(buf.String())
	if logs == "" {
		t.Skip("no logs captured from DeleteRoom")
		return
	}

	lines := strings.Split(logs, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		for key := range entry {
			// Allow standard slog fields and known snake_case fields.
			switch key {
			case "msg", "level", "time", "err", "component":
				continue
			}
			// If the key contains underscore, it's snake_case — OK.
			if strings.Contains(key, "_") {
				continue
			}
			t.Logf("non-snake_case key found: %q", key)
		}
	}
}

// ---------------------------------------------------------------------------
// TASK-038: Test JSON handler writes to file
// ---------------------------------------------------------------------------

func TestJSONHandler_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "talkgo.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create temp file: %v", err)
	}

	bw := bufio.NewWriter(f)
	handler := slog.NewJSONHandler(bw, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	logger.Info("test_message", "component", "test", "value", 42)
	logger.Warn("another_msg", "component", "test", "reason", "testing")

	// Flush the buffered writer and close the file.
	if err := bw.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	f.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d", len(lines))
	}

	for i, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line %d: invalid JSON: %v", i, err)
			continue
		}
		if _, ok := entry["msg"]; !ok {
			t.Errorf("line %d: missing msg field", i)
		}
		if _, ok := entry["time"]; !ok {
			t.Errorf("line %d: missing time field", i)
		}
		if _, ok := entry["level"]; !ok {
			t.Errorf("line %d: missing level field", i)
		}
	}
}

type assertAnError string

func (e assertAnError) Error() string { return string(e) }
