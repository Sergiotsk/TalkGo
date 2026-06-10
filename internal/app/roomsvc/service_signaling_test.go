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

// ---------------------------------------------------------------------------
// DeleteRoom
// ---------------------------------------------------------------------------

func TestService_DeleteRoom_HappyPath(t *testing.T) {
	r := newTestRoom(t)
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		DeleteFn:   func(_ context.Context, _ string) error { return nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := roomsvc.NewService(repo, &mocks.MockWebRTCPeer{})

	err := svc.DeleteRoom(context.Background(), "room-1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !errors.Is(nil, nil) { // trivial guard
		t.Error("unexpected")
	}
	if repo.DeleteCalled != 1 {
		t.Errorf("Delete calls = %d, want 1", repo.DeleteCalled)
	}
	if r.Active {
		t.Error("room should be closed after DeleteRoom")
	}
}

func TestService_DeleteRoom_NotFound(t *testing.T) {
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) {
			return nil, driving.ErrRoomNotFound
		},
	}
	svc := roomsvc.NewService(repo, &mocks.MockWebRTCPeer{})

	err := svc.DeleteRoom(context.Background(), "missing")

	if !errors.Is(err, driving.ErrRoomNotFound) {
		t.Errorf("expected ErrRoomNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// HandleSignaling
// ---------------------------------------------------------------------------

func TestService_HandleSignaling_Join(t *testing.T) {
	r := newTestRoom(t)
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := roomsvc.NewService(repo, &mocks.MockWebRTCPeer{})

	resp, err := svc.HandleSignaling(context.Background(), driving.SignalingMessage{
		Type:   "join",
		RoomID: "room-1",
		UserID: "user-1",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != "joined" {
		t.Errorf("expected type=joined, got %q", resp.Type)
	}
	if resp.SessionID == "" {
		t.Error("expected non-empty session_id in joined response")
	}
}

func TestService_HandleSignaling_Offer(t *testing.T) {
	r := newTestRoom(t)
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	peer := &mocks.MockWebRTCPeer{
		CreateAnswerFn: func(_ context.Context, _ string) (string, error) {
			return "sdp-answer-payload", nil
		},
	}
	svc := roomsvc.NewService(repo, peer)

	// First join to get a session
	sessID, _ := svc.JoinRoom(context.Background(), "room-1", "user-1")

	resp, err := svc.HandleSignaling(context.Background(), driving.SignalingMessage{
		Type:      "offer",
		SessionID: sessID,
		SDP:       "sdp-offer-payload",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != "answer" {
		t.Errorf("expected type=answer, got %q", resp.Type)
	}
	if resp.SDP != "sdp-answer-payload" {
		t.Errorf("expected SDP %q, got %q", "sdp-answer-payload", resp.SDP)
	}
	if peer.HandleOfferCalled != 1 {
		t.Errorf("HandleOffer calls = %d, want 1", peer.HandleOfferCalled)
	}
}

func TestService_HandleSignaling_ICECandidate(t *testing.T) {
	svc := roomsvc.NewService(&mocks.MockRoomRepository{}, &mocks.MockWebRTCPeer{})

	resp, err := svc.HandleSignaling(context.Background(), driving.SignalingMessage{
		Type:      "ice-candidate",
		SessionID: "sess-1",
		Candidate: "candidate:123",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ice-candidate returns empty ACK
	if resp.Type != "" {
		t.Errorf("expected empty ACK type, got %q", resp.Type)
	}
}

func TestService_HandleSignaling_Leave(t *testing.T) {
	r := newTestRoom(t)
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := roomsvc.NewService(repo, &mocks.MockWebRTCPeer{})

	sessID, _ := svc.JoinRoom(context.Background(), "room-1", "user-1")

	resp, err := svc.HandleSignaling(context.Background(), driving.SignalingMessage{
		Type:      "leave",
		SessionID: sessID,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = resp // ACK, no specific type required
}

func TestService_HandleSignaling_UnknownType(t *testing.T) {
	svc := roomsvc.NewService(&mocks.MockRoomRepository{}, &mocks.MockWebRTCPeer{})

	_, err := svc.HandleSignaling(context.Background(), driving.SignalingMessage{Type: "bogus"})

	if !errors.Is(err, driving.ErrUnknownMessageType) {
		t.Errorf("expected ErrUnknownMessageType, got %v", err)
	}
}
