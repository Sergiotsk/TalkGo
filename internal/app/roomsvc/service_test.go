package roomsvc_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Sergiotsk/TalkGo/internal/app/roomsvc"
	"github.com/Sergiotsk/TalkGo/internal/domain/room"
	"github.com/Sergiotsk/TalkGo/internal/ports/driven/mocks"
	"github.com/Sergiotsk/TalkGo/internal/ports/driving"
)

func newTestRoom(t *testing.T) *room.Room {
	t.Helper()
	r, err := room.NewRoom("room-1", "es", "en")
	if err != nil {
		t.Fatalf("creating test room: %v", err)
	}
	return r
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
	svc := roomsvc.NewService(repo, peer)

	sessID, err := svc.JoinRoom(context.Background(), "room-1", "user-1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessID == "" {
		t.Error("expected non-empty sessionID")
	}
	if repo.FindByIDCalled != 1 {
		t.Errorf("FindByID calls = %d, want 1", repo.FindByIDCalled)
	}
	if repo.SaveCalled != 1 {
		t.Errorf("Save calls = %d, want 1", repo.SaveCalled)
	}
	if peer.CreateSessionCalled != 1 {
		t.Errorf("CreateSession calls = %d, want 1", peer.CreateSessionCalled)
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
	svc := roomsvc.NewService(repo, peer)

	_, err := svc.JoinRoom(context.Background(), "nonexistent", "user-1")

	if !errors.Is(err, driving.ErrRoomNotFound) {
		t.Errorf("expected ErrRoomNotFound, got %v", err)
	}
	if peer.CreateSessionCalled != 0 {
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
	svc := roomsvc.NewService(repo, peer)

	_, err := svc.JoinRoom(context.Background(), "room-1", "user-3")

	if !errors.Is(err, room.ErrRoomFull) {
		t.Errorf("expected ErrRoomFull, got %v", err)
	}
	if peer.CreateSessionCalled != 0 {
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
	svc := roomsvc.NewService(repo, peer)

	_, err := svc.JoinRoom(context.Background(), "room-1", "user-1")

	if err == nil {
		t.Fatal("expected error from peer failure, got nil")
	}
	if _, ok := r.Participants["user-1"]; ok {
		t.Error("user-1 should have been removed from room on peer failure (rollback)")
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
	svc := roomsvc.NewService(repo, peer)

	// Join first so we have a session to leave
	sessID, err := svc.JoinRoom(context.Background(), "room-1", "user-1")
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
	if peer.CloseSessionCalled != 1 {
		t.Errorf("CloseSession calls = %d, want 1", peer.CloseSessionCalled)
	}
	if _, ok := r.Participants["user-1"]; ok {
		t.Error("user-1 should have been removed from room after leave")
	}
}

func TestService_LeaveRoom_SessionNotFound(t *testing.T) {
	repo := &mocks.MockRoomRepository{}
	peer := &mocks.MockWebRTCPeer{}
	svc := roomsvc.NewService(repo, peer)

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
	svc := roomsvc.NewService(repo, peer)

	_, err := svc.JoinRoom(context.Background(), "room-1", "user-1")
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
	svc := roomsvc.NewService(repo, &mocks.MockWebRTCPeer{})

	roomID, err := svc.CreateRoom(context.Background(), "es", "en")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if roomID == "" {
		t.Error("expected non-empty roomID")
	}
	if repo.SaveCalled != 1 {
		t.Errorf("Save calls = %d, want 1", repo.SaveCalled)
	}
}

func TestService_CreateRoom_InvalidLanguage(t *testing.T) {
	svc := roomsvc.NewService(&mocks.MockRoomRepository{}, &mocks.MockWebRTCPeer{})

	_, err := svc.CreateRoom(context.Background(), "spa", "en")

	if !errors.Is(err, room.ErrInvalidLanguageCode) {
		t.Errorf("expected ErrInvalidLanguageCode, got %v", err)
	}
}
