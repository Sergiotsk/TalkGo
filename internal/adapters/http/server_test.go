package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	httpserver "github.com/Sergiotsk/TalkGo/internal/adapters/http"
	"github.com/Sergiotsk/TalkGo/internal/domain/room"
	"github.com/Sergiotsk/TalkGo/internal/ports/driving"
)

// mockRoomManager is a minimal test double for driving.RoomManager.
type mockRoomManager struct {
	createRoomFn      func(ctx context.Context, src, tgt string) (driving.CreateRoomResult, error)
	deleteRoomFn      func(ctx context.Context, roomID string) error
	roomExistsFn      func(ctx context.Context, roomID string) error
	findByShortCodeFn func(ctx context.Context, code string) (*room.Room, error)
	updateActivityFn  func(ctx context.Context, roomID string) error
}

func (m *mockRoomManager) CreateRoom(ctx context.Context, src, tgt string) (driving.CreateRoomResult, error) {
	if m.createRoomFn != nil {
		return m.createRoomFn(ctx, src, tgt)
	}
	r, _ := room.NewRoom("room-uuid", src, tgt)
	r.ShortCode = "ABCD12"
	return driving.CreateRoomResult{Room: r}, nil
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
func (m *mockRoomManager) JoinRoom(_ context.Context, _, _, _ string) (string, error) { return "", nil }
func (m *mockRoomManager) LeaveRoom(_ context.Context, _, _ string) error             { return nil }
func (m *mockRoomManager) FindByShortCode(ctx context.Context, code string) (*room.Room, error) {
	if m.findByShortCodeFn != nil {
		return m.findByShortCodeFn(ctx, code)
	}
	return nil, driving.ErrRoomNotFound
}
func (m *mockRoomManager) UpdateLastActivity(ctx context.Context, roomID string) error {
	if m.updateActivityFn != nil {
		return m.updateActivityFn(ctx, roomID)
	}
	return nil
}

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
		createRoomFn: func(_ context.Context, _, _ string) (driving.CreateRoomResult, error) {
			r, _ := room.NewRoom("new-room-id", "es", "en")
			r.ShortCode = "XYZABC"
			return driving.CreateRoomResult{Room: r}, nil
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
	if resp["short_code"] != "XYZABC" {
		t.Errorf(`short_code = %q, want "XYZABC"`, resp["short_code"])
	}
}

func TestCreateRoomHandler_BadRequest(t *testing.T) {
	mgr := &mockRoomManager{
		createRoomFn: func(_ context.Context, _, _ string) (driving.CreateRoomResult, error) {
			return driving.CreateRoomResult{}, fmt.Errorf("roomsvc.CreateRoom: %w", room.ErrInvalidLanguageCode)
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

// ---------------------------------------------------------------------------
// GET /rooms/code/{code}
// ---------------------------------------------------------------------------

func TestGetRoomByShortCode_Found(t *testing.T) {
	r, _ := room.NewRoom("room-sc-1", "es", "en")
	r.ShortCode = "ABCDEF"
	mgr := &mockRoomManager{
		findByShortCodeFn: func(_ context.Context, _ string) (*room.Room, error) {
			return r, nil
		},
	}
	srv := httpserver.NewServer(httpserver.DefaultConfig(), mgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/rooms/code/ABCDEF", http.NoBody)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["room_id"] != "room-sc-1" {
		t.Errorf(`room_id = %q, want "room-sc-1"`, resp["room_id"])
	}
	if resp["short_code"] != "ABCDEF" {
		t.Errorf(`short_code = %q, want "ABCDEF"`, resp["short_code"])
	}
}

func TestGetRoomByShortCode_NotFound(t *testing.T) {
	mgr := &mockRoomManager{
		findByShortCodeFn: func(_ context.Context, _ string) (*room.Room, error) {
			return nil, driving.ErrRoomNotFound
		},
	}
	srv := httpserver.NewServer(httpserver.DefaultConfig(), mgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/rooms/code/ZZZZZZ", http.NoBody)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetRoomByShortCode_Expired(t *testing.T) {
	mgr := &mockRoomManager{
		findByShortCodeFn: func(_ context.Context, _ string) (*room.Room, error) {
			return nil, room.ErrRoomClosed
		},
	}
	srv := httpserver.NewServer(httpserver.DefaultConfig(), mgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/rooms/code/EXPIRY", http.NoBody)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Errorf("status = %d, want %d", w.Code, http.StatusGone)
	}
}

// ---------------------------------------------------------------------------
// POST /rooms — 409 room full
// ---------------------------------------------------------------------------

func TestCreateRoomHandler_409_RoomFull(t *testing.T) {
	mgr := &mockRoomManager{
		createRoomFn: func(_ context.Context, _, _ string) (driving.CreateRoomResult, error) {
			return driving.CreateRoomResult{}, room.ErrRoomFull
		},
	}
	srv := httpserver.NewServer(httpserver.DefaultConfig(), mgr, nil)

	body := bytes.NewBufferString(`{"source_lang":"es","target_lang":"en"}`)
	req := httptest.NewRequest(http.MethodPost, "/rooms", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

// ---------------------------------------------------------------------------
// POST /rooms — 410 room closed/expired
// ---------------------------------------------------------------------------

func TestCreateRoomHandler_410_RoomClosed(t *testing.T) {
	mgr := &mockRoomManager{
		createRoomFn: func(_ context.Context, _, _ string) (driving.CreateRoomResult, error) {
			return driving.CreateRoomResult{}, room.ErrRoomClosed
		},
	}
	srv := httpserver.NewServer(httpserver.DefaultConfig(), mgr, nil)

	body := bytes.NewBufferString(`{"source_lang":"es","target_lang":"en"}`)
	req := httptest.NewRequest(http.MethodPost, "/rooms", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Errorf("status = %d, want %d", w.Code, http.StatusGone)
	}
}

// ---------------------------------------------------------------------------
// POST /rooms — 500 internal server error (unexpected error)
// ---------------------------------------------------------------------------

func TestCreateRoomHandler_500_InternalError(t *testing.T) {
	mgr := &mockRoomManager{
		createRoomFn: func(_ context.Context, _, _ string) (driving.CreateRoomResult, error) {
			return driving.CreateRoomResult{}, fmt.Errorf("unexpected database error")
		},
	}
	srv := httpserver.NewServer(httpserver.DefaultConfig(), mgr, nil)

	body := bytes.NewBufferString(`{"source_lang":"es","target_lang":"en"}`)
	req := httptest.NewRequest(http.MethodPost, "/rooms", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// DELETE /rooms/{id} — 500 internal server error (unexpected error)
// ---------------------------------------------------------------------------

func TestDeleteRoomHandler_500_InternalError(t *testing.T) {
	mgr := &mockRoomManager{
		deleteRoomFn: func(_ context.Context, _ string) error {
			return fmt.Errorf("unexpected database error")
		},
	}
	srv := httpserver.NewServer(httpserver.DefaultConfig(), mgr, nil)

	req := httptest.NewRequest(http.MethodDelete, "/rooms/room-123", http.NoBody)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// GET /rooms/code/{code} — 500 internal server error (unexpected error)
// ---------------------------------------------------------------------------

func TestGetRoomByShortCode_500_InternalError(t *testing.T) {
	mgr := &mockRoomManager{
		findByShortCodeFn: func(_ context.Context, _ string) (*room.Room, error) {
			return nil, fmt.Errorf("unexpected database error")
		},
	}
	srv := httpserver.NewServer(httpserver.DefaultConfig(), mgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/rooms/code/SHORT", http.NoBody)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// GET /ws/{roomID} — RoomExists returns generic error → 500
// ---------------------------------------------------------------------------

func TestWSHandler_RoomExistsError(t *testing.T) {
	mgr := &mockRoomManager{
		roomExistsFn: func(_ context.Context, _ string) error {
			return fmt.Errorf("some unexpected db error")
		},
	}
	srv := httpserver.NewServer(httpserver.DefaultConfig(), mgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/ws/room-123", http.NoBody)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// GET /ws/{roomID} — hub is nil → 500
// ---------------------------------------------------------------------------

func TestWSHandler_NilHub(t *testing.T) {
	srv := httpserver.NewServer(httpserver.DefaultConfig(), &mockRoomManager{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/ws/room-123", http.NoBody)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// ListenAndServe — signal-based shutdown
// ---------------------------------------------------------------------------

func TestListenAndServe_StartsListening(t *testing.T) {
	cfg := httpserver.DefaultConfig()
	cfg.Addr = "127.0.0.1:0"

	srv := httpserver.NewServer(cfg, &mockRoomManager{}, nil)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe(context.Background())
	}()

	// If the server fails to start, errCh receives immediately.
	// Otherwise, ListenAndServe blocks waiting for OS signal.
	select {
	case err := <-errCh:
		t.Fatalf("server unexpectedly failed to start: %v", err)
	case <-time.After(200 * time.Millisecond):
		// Server started successfully — the goroutine is now
		// blocked inside ListenAndServe waiting for SIGINT/SIGTERM.
	}
}
