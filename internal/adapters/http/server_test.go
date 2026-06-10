package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	httpserver "github.com/Sergiotsk/TalkGo/internal/adapters/http"
	"github.com/Sergiotsk/TalkGo/internal/domain/room"
	"github.com/Sergiotsk/TalkGo/internal/ports/driving"
)

// mockRoomManager is a minimal test double for driving.RoomManager.
type mockRoomManager struct {
	createRoomFn func(ctx context.Context, src, tgt string) (string, error)
	deleteRoomFn func(ctx context.Context, roomID string) error
	roomExistsFn func(ctx context.Context, roomID string) error
}

func (m *mockRoomManager) CreateRoom(ctx context.Context, src, tgt string) (string, error) {
	if m.createRoomFn != nil {
		return m.createRoomFn(ctx, src, tgt)
	}
	return "room-uuid", nil
}
func (m *mockRoomManager) DeleteRoom(ctx context.Context, id string) error {
	if m.deleteRoomFn != nil {
		return m.deleteRoomFn(ctx, id)
	}
	return nil
}
func (m *mockRoomManager) RoomExists(ctx context.Context, roomID string) error {
	if m.roomExistsFn != nil {
		return m.roomExistsFn(ctx, roomID)
	}
	return nil
}
func (m *mockRoomManager) JoinRoom(_ context.Context, _, _ string) (string, error) { return "", nil }
func (m *mockRoomManager) LeaveRoom(_ context.Context, _, _ string) error          { return nil }

// ---------------------------------------------------------------------------
// GET /health
// ---------------------------------------------------------------------------

func TestHealthHandler(t *testing.T) {
	srv := httpserver.NewServer(httpserver.DefaultConfig(), &mockRoomManager{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", http.NoBody)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf(`body["status"] = %q, want "ok"`, body["status"])
	}
}

// ---------------------------------------------------------------------------
// POST /rooms
// ---------------------------------------------------------------------------

func TestCreateRoomHandler_Created(t *testing.T) {
	mgr := &mockRoomManager{
		createRoomFn: func(_ context.Context, _, _ string) (string, error) {
			return "new-room-id", nil
		},
	}
	srv := httpserver.NewServer(httpserver.DefaultConfig(), mgr, nil)

	body := bytes.NewBufferString(`{"source_lang":"es","target_lang":"en"}`)
	req := httptest.NewRequest(http.MethodPost, "/rooms", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["room_id"] != "new-room-id" {
		t.Errorf(`room_id = %q, want "new-room-id"`, resp["room_id"])
	}
}

func TestCreateRoomHandler_BadRequest(t *testing.T) {
	mgr := &mockRoomManager{
		createRoomFn: func(_ context.Context, _, _ string) (string, error) {
			return "", fmt.Errorf("roomsvc.CreateRoom: %w", room.ErrInvalidLanguageCode)
		},
	}
	srv := httpserver.NewServer(httpserver.DefaultConfig(), mgr, nil)

	body := bytes.NewBufferString(`{"source_lang":"spa","target_lang":"en"}`)
	req := httptest.NewRequest(http.MethodPost, "/rooms", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateRoomHandler_InvalidJSON(t *testing.T) {
	srv := httpserver.NewServer(httpserver.DefaultConfig(), &mockRoomManager{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/rooms", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// DELETE /rooms/{id}
// ---------------------------------------------------------------------------

func TestDeleteRoomHandler_NoContent(t *testing.T) {
	srv := httpserver.NewServer(httpserver.DefaultConfig(), &mockRoomManager{}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/rooms/room-123", http.NoBody)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestDeleteRoomHandler_NotFound(t *testing.T) {
	mgr := &mockRoomManager{
		deleteRoomFn: func(_ context.Context, _ string) error {
			return driving.ErrRoomNotFound
		},
	}
	srv := httpserver.NewServer(httpserver.DefaultConfig(), mgr, nil)

	req := httptest.NewRequest(http.MethodDelete, "/rooms/missing", http.NoBody)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// GET /ws/{roomID}
// ---------------------------------------------------------------------------

func TestWSHandler_RoomNotFound(t *testing.T) {
	mgr := &mockRoomManager{
		roomExistsFn: func(_ context.Context, _ string) error {
			return driving.ErrRoomNotFound
		},
	}
	srv := httpserver.NewServer(httpserver.DefaultConfig(), mgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/ws/nonexistent-room", http.NoBody)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
