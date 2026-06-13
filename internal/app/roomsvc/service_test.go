package roomsvc_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Sergiotsk/TalkGo/internal/app/roomsvc"
	"github.com/Sergiotsk/TalkGo/internal/domain/room"
	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
	"github.com/Sergiotsk/TalkGo/internal/ports/driven/mocks"
	"github.com/Sergiotsk/TalkGo/internal/ports/driving"
)

// newDefaultService builds a Service with no-op mocks for the three new ports.
func newDefaultService(t *testing.T, repo driven.RoomRepository, peer driven.WebRTCPeer) *roomsvc.Service {
	t.Helper()
	cfg := roomsvc.ServiceConfig{
		GracePeriod:         1 * time.Millisecond,
		RoomTTL:             10 * time.Minute,
		SweepInterval:       1 * time.Hour,
		MaxShortCodeRetries: 5,
	}
	svc, err := roomsvc.NewService(cfg, repo, peer, &mocks.MockTranslator{}, &mocks.MockAudioCodec{}, &mocks.MockEventNotifier{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func newTestRoom(t *testing.T) *room.Room {
	t.Helper()
	r, err := room.NewRoom("room-1", "es", "en")
	if err != nil {
		t.Fatalf("creating test room: %v", err)
	}
	return r
}

// ---------------------------------------------------------------------------
// NewService — nil dependency guard
// ---------------------------------------------------------------------------

func TestNewService_NilDependency(t *testing.T) {
	repo := &mocks.MockRoomRepository{}
	peer := &mocks.MockWebRTCPeer{}
	codec := &mocks.MockAudioCodec{}
	notifier := &mocks.MockEventNotifier{}
	translator := &mocks.MockTranslator{}

	tests := []struct {
		name       string
		repo       driven.RoomRepository
		peer       driven.WebRTCPeer
		translator driven.Translator
		codec      driven.AudioCodec
		notifier   driven.EventNotifier
	}{
		{"nil repo", nil, peer, translator, codec, notifier},
		{"nil peer", repo, nil, translator, codec, notifier},
		{"nil translator", repo, peer, nil, codec, notifier},
		{"nil codec", repo, peer, translator, nil, notifier},
		{"nil notifier", repo, peer, translator, codec, nil},
	}
	cfg := roomsvc.ServiceConfig{MaxShortCodeRetries: 5}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := roomsvc.NewService(cfg, tt.repo, tt.peer, tt.translator, tt.codec, tt.notifier)
			if !errors.Is(err, roomsvc.ErrNilDependency) {
				t.Errorf("expected ErrNilDependency, got %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// JoinRoom
// ---------------------------------------------------------------------------

func TestService_JoinRoom_HappyPath(t *testing.T) {
	r := newTestRoom(t)
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	peer := &mocks.MockWebRTCPeer{}
	svc := newDefaultService(t, repo, peer)

	sessID, err := svc.JoinRoom(context.Background(), "room-1", "user-1", "es")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessID == "" {
		t.Error("expected non-empty sessionID")
	}
	if repo.FindByIDCalled != 1 {
		t.Errorf("FindByID calls = %d, want 1", repo.FindByIDCalled)
	}
	if repo.SaveCalled < 1 {
		t.Errorf("Save calls = %d, want ≥1", repo.SaveCalled)
	}
	if peer.CreateSessionCalled() != 1 {
		t.Errorf("CreateSession calls = %d, want 1", peer.CreateSessionCalled())
	}
	if _, ok := r.Participants["user-1"]; !ok {
		t.Error("expected user-1 in room participants after join")
	}
}

func TestService_JoinRoom_RoomNotFound(t *testing.T) {
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) {
			return nil, driving.ErrRoomNotFound
		},
	}
	peer := &mocks.MockWebRTCPeer{}
	svc := newDefaultService(t, repo, peer)

	_, err := svc.JoinRoom(context.Background(), "nonexistent", "user-1", "es")

	if !errors.Is(err, driving.ErrRoomNotFound) {
		t.Errorf("expected ErrRoomNotFound, got %v", err)
	}
	if peer.CreateSessionCalled() != 0 {
		t.Error("CreateSession must NOT be called when room not found")
	}
}

func TestService_JoinRoom_RoomFull(t *testing.T) {
	r := newTestRoom(t)
	_ = r.Join("user-1")
	_ = r.Join("user-2")

	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
	}
	peer := &mocks.MockWebRTCPeer{}
	svc := newDefaultService(t, repo, peer)

	_, err := svc.JoinRoom(context.Background(), "room-1", "user-3", "es")

	if !errors.Is(err, room.ErrRoomFull) {
		t.Errorf("expected ErrRoomFull, got %v", err)
	}
	if peer.CreateSessionCalled() != 0 {
		t.Error("CreateSession must NOT be called when room is full")
	}
}

func TestService_JoinRoom_PeerError_Rollback(t *testing.T) {
	r := newTestRoom(t)
	peerErr := errors.New("pion: failed to create peer connection")
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	peer := &mocks.MockWebRTCPeer{
		CreateSessionFn: func(_ context.Context, _ string) error { return peerErr },
	}
	svc := newDefaultService(t, repo, peer)

	_, err := svc.JoinRoom(context.Background(), "room-1", "user-1", "es")

	if err == nil {
		t.Fatal("expected error from peer failure, got nil")
	}
	if _, ok := r.Participants["user-1"]; ok {
		t.Error("user-1 should have been removed from room on peer failure (rollback)")
	}
}

func TestService_JoinRoom_MissingLang(t *testing.T) {
	r := newTestRoom(t)
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
	}
	svc := newDefaultService(t, repo, &mocks.MockWebRTCPeer{})

	_, err := svc.JoinRoom(context.Background(), "room-1", "user-1", "")

	if !errors.Is(err, roomsvc.ErrMissingLang) {
		t.Errorf("expected ErrMissingLang, got %v", err)
	}
}

func TestService_JoinRoom_LangNotSupported(t *testing.T) {
	r := newTestRoom(t) // SourceLang="es", TargetLang="en"
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
	}
	svc := newDefaultService(t, repo, &mocks.MockWebRTCPeer{})

	_, err := svc.JoinRoom(context.Background(), "room-1", "user-1", "fr")

	if !errors.Is(err, roomsvc.ErrLangNotSupported) {
		t.Errorf("expected ErrLangNotSupported, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// LeaveRoom
// ---------------------------------------------------------------------------

func TestService_LeaveRoom_HappyPath(t *testing.T) {
	r := newTestRoom(t)
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	peer := &mocks.MockWebRTCPeer{}
	svc := newDefaultService(t, repo, peer)

	// Join first so we have a session to leave
	sessID, err := svc.JoinRoom(context.Background(), "room-1", "user-1", "es")
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if sessID == "" {
		t.Fatal("expected sessionID from join")
	}

	err = svc.LeaveRoom(context.Background(), "room-1", "user-1")

	if err != nil {
		t.Fatalf("LeaveRoom unexpected error: %v", err)
	}
	if peer.CloseSessionCalled() != 1 {
		t.Errorf("CloseSession calls = %d, want 1", peer.CloseSessionCalled())
	}
	if _, ok := r.Participants["user-1"]; ok {
		t.Error("user-1 should have been removed from room after leave")
	}
}

func TestService_LeaveRoom_SessionNotFound(t *testing.T) {
	repo := &mocks.MockRoomRepository{}
	peer := &mocks.MockWebRTCPeer{}
	svc := newDefaultService(t, repo, peer)

	err := svc.LeaveRoom(context.Background(), "room-1", "user-99")

	if !errors.Is(err, driving.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestService_LeaveRoom_PeerCloseError_NonFatal(t *testing.T) {
	r := newTestRoom(t)
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	peer := &mocks.MockWebRTCPeer{
		CloseSessionFn: func(_ context.Context, _ string) error {
			return errors.New("pion: close failed")
		},
	}
	svc := newDefaultService(t, repo, peer)

	_, err := svc.JoinRoom(context.Background(), "room-1", "user-1", "es")
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	// Peer close error must NOT propagate — Leave should succeed
	err = svc.LeaveRoom(context.Background(), "room-1", "user-1")
	if err != nil {
		t.Errorf("LeaveRoom should succeed even when peer.CloseSession fails, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CreateRoom
// ---------------------------------------------------------------------------

func TestService_CreateRoom_HappyPath(t *testing.T) {
	repo := &mocks.MockRoomRepository{
		SaveFn: func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := newDefaultService(t, repo, &mocks.MockWebRTCPeer{})

	result, err := svc.CreateRoom(context.Background(), "es", "en")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Room == nil {
		t.Fatal("expected non-nil Room in result")
	}
	if result.Room.ID == "" {
		t.Error("expected non-empty roomID")
	}
	if repo.SaveCalled < 1 {
		t.Errorf("Save calls = %d, want ≥1", repo.SaveCalled)
	}
}

func TestService_CreateRoom_InvalidLanguage(t *testing.T) {
	svc := newDefaultService(t, &mocks.MockRoomRepository{}, &mocks.MockWebRTCPeer{})

	_, err := svc.CreateRoom(context.Background(), "spa", "en")

	if !errors.Is(err, room.ErrInvalidLanguageCode) {
		t.Errorf("expected ErrInvalidLanguageCode, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// TASK-031: LeaveRoom emits session_end (voluntary)
// ---------------------------------------------------------------------------

func TestService_LeaveRoom_EmitsSessionEnd(t *testing.T) {
	r, _ := room.NewRoom("room-1", "es", "en")
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := newDefaultService(t, repo, &mocks.MockWebRTCPeer{})

	buf, restore := captureSlog(t)
	defer restore()

	ctx := context.Background()
	_, err := svc.JoinRoom(ctx, "room-1", "user-1", "es")
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	if err := svc.LeaveRoom(ctx, "room-1", "user-1"); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}

	// Parse logged events.
	logs := buf.String()
	var found bool
	dec := json.NewDecoder(strings.NewReader(logs))
	for dec.More() {
		var entry map[string]any
		if err := dec.Decode(&entry); err != nil {
			break
		}
		msg, _ := entry["msg"].(string)
		if msg != "session_event" {
			continue
		}
		evt, _ := entry["event"].(string)
		if evt != "session_end" {
			continue
		}
		found = true

		// Validate required fields.
		if _, ok := entry["session_id"]; !ok {
			t.Error("session_end missing session_id")
		}
		if _, ok := entry["room_id"]; !ok {
			t.Error("session_end missing room_id")
		}
		dur, ok := entry["duration_sec"]
		if !ok {
			t.Error("session_end missing duration_sec")
		} else if _, isFloat := dur.(float64); !isFloat {
			t.Errorf("duration_sec is not a number, got %T", dur)
		}
		et, _ := entry["event_type"].(string)
		if et != "voluntary" {
			t.Errorf("event_type = %q, want %q", et, "voluntary")
		}
		if sessID, _ := entry["session_id"].(string); sessID == "" {
			t.Error("session_id should not be empty")
		}
	}
	if !found {
		t.Fatal("expected session_end event with event_type 'voluntary', got none")
	}
}

func TestService_LeaveRoom_EmitsSessionEnd_SessionIDNotEmpty(t *testing.T) {
	r, _ := room.NewRoom("room-1", "es", "en")
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := newDefaultService(t, repo, &mocks.MockWebRTCPeer{})

	buf, restore := captureSlog(t)
	defer restore()

	ctx := context.Background()
	expectedSessID, err := svc.JoinRoom(ctx, "room-1", "user-1", "es")
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	if err := svc.LeaveRoom(ctx, "room-1", "user-1"); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}

	logs := buf.String()
	dec := json.NewDecoder(strings.NewReader(logs))
	for dec.More() {
		var entry map[string]any
		if err := dec.Decode(&entry); err != nil {
			break
		}
		msg, _ := entry["msg"].(string)
		if msg != "session_event" {
			continue
		}
		evt, _ := entry["event"].(string)
		if evt != "session_end" {
			continue
		}
		gotID, _ := entry["session_id"].(string)
		if gotID != expectedSessID {
			t.Errorf("session_id = %q, want %q", gotID, expectedSessID)
		}
		return
	}
	t.Fatal("expected session_end event, got none")
}

// ---------------------------------------------------------------------------
// TASK-033: OnDisconnect emits session_end (disconnect)
// ---------------------------------------------------------------------------

func TestService_OnDisconnect_EmitsSessionEnd(t *testing.T) {
	r, _ := room.NewRoom("room-1", "es", "en")
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := newDefaultService(t, repo, &mocks.MockWebRTCPeer{})

	buf, restore := captureSlog(t)
	defer restore()

	ctx := context.Background()
	sessID, err := svc.JoinRoom(ctx, "room-1", "user-1", "es")
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	if err := svc.OnDisconnect(ctx, sessID); err != nil {
		t.Fatalf("OnDisconnect: %v", err)
	}

	logs := buf.String()
	var found bool
	dec := json.NewDecoder(strings.NewReader(logs))
	for dec.More() {
		var entry map[string]any
		if err := dec.Decode(&entry); err != nil {
			break
		}
		msg, _ := entry["msg"].(string)
		if msg != "session_event" {
			continue
		}
		evt, _ := entry["event"].(string)
		if evt != "session_end" {
			continue
		}
		et, _ := entry["event_type"].(string)
		if et == "disconnect" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected session_end event with event_type 'disconnect', got none")
	}
}

func TestService_OnDisconnect_EmitsSessionEnd_EmptySessionIsNoOp(t *testing.T) {
	svc := newDefaultService(t, &mocks.MockRoomRepository{}, &mocks.MockWebRTCPeer{})

	buf, restore := captureSlog(t)
	defer restore()

	if err := svc.OnDisconnect(context.Background(), ""); err != nil {
		t.Fatalf("OnDisconnect with empty sessionID: %v", err)
	}

	if buf.Len() > 0 {
		t.Error("expected no log output for empty sessionID")
	}
}

func TestService_OnDisconnect_EmitsSessionEnd_UnknownSessionIsNoOp(t *testing.T) {
	svc := newDefaultService(t, &mocks.MockRoomRepository{}, &mocks.MockWebRTCPeer{})

	buf, restore := captureSlog(t)
	defer restore()

	if err := svc.OnDisconnect(context.Background(), "unknown-session"); err != nil {
		t.Fatalf("OnDisconnect with unknown sessionID: %v", err)
	}

	if buf.Len() > 0 {
		t.Error("expected no log output for unknown sessionID")
	}
}

// ---------------------------------------------------------------------------
// TASK-041: Edge case — unsupported language pair (should not panic)
// ---------------------------------------------------------------------------

func TestService_CreateRoom_InvalidLangCodes(t *testing.T) {
	svc := newDefaultService(t, &mocks.MockRoomRepository{}, &mocks.MockWebRTCPeer{})

	tests := []struct {
		name string
		src  string
		tgt  string
	}{
		{"empty src", "", "en"},
		{"empty tgt", "es", ""},
		{"too long src", "espanol", "en"},
		{"too long tgt", "es", "english"},
		{"single char", "e", "n"},
		{"both empty", "", ""},
	}
	// Note: 2-char codes like "xx" are valid per ISO 639-1 length validation.
	// The domain intentionally accepts any 2-char code; language support is
	// enforced at the translator/service level (ErrLangNotSupported).
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateRoom(context.Background(), tt.src, tt.tgt)
			if err == nil {
				t.Fatal("expected error for invalid language codes, got nil")
			}
			t.Logf("got expected error: %v", err)
		})
	}
}

// ---------------------------------------------------------------------------
// TASK-042: Edge case — rapid join/leave cycles without deadlock
// ---------------------------------------------------------------------------

func TestService_RapidJoinLeaveCycles(t *testing.T) {
	r, _ := room.NewRoom("room-rapid", "es", "en")
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := newDefaultService(t, repo, &mocks.MockWebRTCPeer{})

	ctx := context.Background()
	const cycles = 10

	for i := 0; i < cycles; i++ {
		sessID, err := svc.JoinRoom(ctx, "room-rapid", "user-1", "es")
		if err != nil {
			t.Fatalf("cycle %d: JoinRoom: %v", i, err)
		}
		if sessID == "" {
			t.Fatalf("cycle %d: empty session ID", i)
		}
		if err := svc.LeaveRoom(ctx, "room-rapid", "user-1"); err != nil {
			t.Fatalf("cycle %d: LeaveRoom: %v", i, err)
		}
	}
	t.Logf("completed %d join/leave cycles without deadlock", cycles)
}
