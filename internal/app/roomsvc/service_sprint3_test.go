package roomsvc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sergiotsk/TalkGo/internal/app/roomsvc"
	"github.com/Sergiotsk/TalkGo/internal/domain/room"
	"github.com/Sergiotsk/TalkGo/internal/ports/driven/mocks"
	"github.com/Sergiotsk/TalkGo/internal/ports/driving"
)

// newServiceWithConfig creates a Service with the given config and mocks.
func newServiceWithConfig(t *testing.T, cfg roomsvc.ServiceConfig, repo *mocks.MockRoomRepository, peer *mocks.MockWebRTCPeer) *roomsvc.Service {
	t.Helper()
	notifier := &mocks.MockEventNotifier{}
	svc, err := roomsvc.NewService(cfg, repo, peer, &mocks.MockTranslator{}, &mocks.MockAudioCodec{}, notifier)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func defaultCfg() roomsvc.ServiceConfig {
	return roomsvc.ServiceConfig{
		GracePeriod:         1 * time.Millisecond,
		RoomTTL:             10 * time.Minute,
		SweepInterval:       1 * time.Hour,
		MaxShortCodeRetries: 5,
	}
}

// ---------------------------------------------------------------------------
// ServiceConfig — NewService signature change
// ---------------------------------------------------------------------------

func TestNewService_WithConfig_NilDependency(t *testing.T) {
	cfg := defaultCfg()
	peer := &mocks.MockWebRTCPeer{}
	translator := &mocks.MockTranslator{}
	codec := &mocks.MockAudioCodec{}
	notifier := &mocks.MockEventNotifier{}

	_, err := roomsvc.NewService(cfg, nil, peer, translator, codec, notifier)
	if !errors.Is(err, roomsvc.ErrNilDependency) {
		t.Errorf("expected ErrNilDependency for nil repo, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// CreateRoom — now returns CreateRoomResult with ShortCode
// ---------------------------------------------------------------------------

func TestService_CreateRoom_ReturnsShortCode(t *testing.T) {
	repo := &mocks.MockRoomRepository{
		SaveFn: func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := newServiceWithConfig(t, defaultCfg(), repo, &mocks.MockWebRTCPeer{})

	result, err := svc.CreateRoom(context.Background(), "es", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Room == nil {
		t.Fatal("expected non-nil Room in result")
	}
	if result.Room.ID == "" {
		t.Error("expected non-empty room ID")
	}
	if len(result.Room.ShortCode) != 6 {
		t.Errorf("expected 6-char short code, got %q (len=%d)", result.Room.ShortCode, len(result.Room.ShortCode))
	}
}

func TestService_CreateRoom_ShortCodeRetryOnCollision(t *testing.T) {
	callCount := 0
	repo := &mocks.MockRoomRepository{
		// First FindByShortCode call returns a room (collision), second returns not found
		FindByShortCodeFn: func(_ context.Context, _ string) (*room.Room, error) {
			callCount++
			if callCount == 1 {
				r, _ := room.NewRoom("existing", "es", "en")
				return r, nil // collision
			}
			return nil, driving.ErrRoomNotFound // no collision
		},
		SaveFn: func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := newServiceWithConfig(t, defaultCfg(), repo, &mocks.MockWebRTCPeer{})

	result, err := svc.CreateRoom(context.Background(), "es", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Room == nil {
		t.Fatal("expected non-nil Room")
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 FindByShortCode calls (retry), got %d", callCount)
	}
}

func TestService_CreateRoom_ExhaustedRetries(t *testing.T) {
	repo := &mocks.MockRoomRepository{
		// Always return collision
		FindByShortCodeFn: func(_ context.Context, _ string) (*room.Room, error) {
			r, _ := room.NewRoom("existing", "es", "en")
			return r, nil
		},
		SaveFn: func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := newServiceWithConfig(t, defaultCfg(), repo, &mocks.MockWebRTCPeer{})

	_, err := svc.CreateRoom(context.Background(), "es", "en")
	if !errors.Is(err, room.ErrShortCodeExhausted) {
		t.Errorf("expected ErrShortCodeExhausted after 5 retries, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// FindByShortCode
// ---------------------------------------------------------------------------

func TestService_FindByShortCode_HappyPath(t *testing.T) {
	r, _ := room.NewRoom("room-1", "es", "en")
	r.ShortCode = "ABCDEF"
	repo := &mocks.MockRoomRepository{
		FindByShortCodeFn: func(_ context.Context, _ string) (*room.Room, error) {
			return r, nil
		},
	}
	svc := newServiceWithConfig(t, defaultCfg(), repo, &mocks.MockWebRTCPeer{})

	got, err := svc.FindByShortCode(context.Background(), "ABCDEF")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "room-1" {
		t.Errorf("got ID %q, want %q", got.ID, "room-1")
	}
}

func TestService_FindByShortCode_NotFound(t *testing.T) {
	repo := &mocks.MockRoomRepository{
		FindByShortCodeFn: func(_ context.Context, _ string) (*room.Room, error) {
			return nil, driving.ErrRoomNotFound
		},
	}
	svc := newServiceWithConfig(t, defaultCfg(), repo, &mocks.MockWebRTCPeer{})

	_, err := svc.FindByShortCode(context.Background(), "ZZZZZZ")
	if !errors.Is(err, driving.ErrRoomNotFound) {
		t.Errorf("expected ErrRoomNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// OnDisconnect — grace period
// ---------------------------------------------------------------------------

func TestService_OnDisconnect_EmptySessionID_NoOp(t *testing.T) {
	repo := &mocks.MockRoomRepository{}
	svc := newServiceWithConfig(t, defaultCfg(), repo, &mocks.MockWebRTCPeer{})

	err := svc.OnDisconnect(context.Background(), "")
	if err != nil {
		t.Errorf("OnDisconnect with empty sessionID should be no-op, got %v", err)
	}
}

func TestService_OnDisconnect_UnknownSession_NoOp(t *testing.T) {
	repo := &mocks.MockRoomRepository{}
	svc := newServiceWithConfig(t, defaultCfg(), repo, &mocks.MockWebRTCPeer{})

	// Unknown session should not return error (no-op per spec)
	err := svc.OnDisconnect(context.Background(), "nonexistent-session")
	if err != nil {
		t.Errorf("OnDisconnect with unknown sessionID should be no-op, got %v", err)
	}
}

func TestService_OnDisconnect_StartsGraceTimer(t *testing.T) {
	r, _ := room.NewRoom("room-1", "es", "en")

	deleted := make(chan string, 1)
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		DeleteFn: func(_ context.Context, id string) error {
			select {
			case deleted <- id:
			default:
			}
			return nil
		},
		SaveFn: func(_ context.Context, _ *room.Room) error { return nil },
	}

	cfg := roomsvc.ServiceConfig{
		GracePeriod:         10 * time.Millisecond,
		RoomTTL:             10 * time.Minute,
		SweepInterval:       1 * time.Hour,
		MaxShortCodeRetries: 5,
	}
	svc := newServiceWithConfig(t, cfg, repo, &mocks.MockWebRTCPeer{})

	// Join to create a session
	sessID, err := svc.JoinRoom(context.Background(), "room-1", "user-1", "es")
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	// Disconnect — should start grace timer
	_ = svc.OnDisconnect(context.Background(), sessID)

	// Wait for grace timer to fire
	select {
	case id := <-deleted:
		if id != "room-1" {
			t.Errorf("expected room-1 to be deleted, got %q", id)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("grace timer did not fire and delete room within timeout")
	}
}

func TestService_OnDisconnect_GraceTimerCancelledOnRejoin(t *testing.T) {
	r, _ := room.NewRoom("room-1", "es", "en")

	var deleteCalled bool
	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
		DeleteFn: func(_ context.Context, _ string) error {
			deleteCalled = true
			return nil
		},
	}

	cfg := roomsvc.ServiceConfig{
		GracePeriod:         20 * time.Millisecond,
		RoomTTL:             10 * time.Minute,
		SweepInterval:       1 * time.Hour,
		MaxShortCodeRetries: 5,
	}
	svc := newServiceWithConfig(t, cfg, repo, &mocks.MockWebRTCPeer{})

	// Join, disconnect, rejoin before grace period expires
	sessID, err := svc.JoinRoom(context.Background(), "room-1", "user-1", "es")
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	// Disconnect — starts grace timer
	_ = svc.OnDisconnect(context.Background(), sessID)

	// Rejoin immediately — should cancel the grace timer
	_, err = svc.JoinRoom(context.Background(), "room-1", "user-1", "es")
	// user-1 is already in room (from first join, we didn't remove participant)
	// The important thing is the timer gets cancelled — we check deleteCalled after waiting
	_ = err

	// Wait longer than grace period
	time.Sleep(40 * time.Millisecond)

	if deleteCalled {
		t.Error("DeleteRoom should NOT have been called — grace timer should have been cancelled on rejoin")
	}
}

// ---------------------------------------------------------------------------
// startExpirationSweep
// ---------------------------------------------------------------------------

func TestService_StartExpirationSweep_DeletesExpiredRooms(t *testing.T) {
	r, _ := room.NewRoom("expired-room", "es", "en")
	r.LastActivity = time.Now().Add(-20 * time.Minute)

	deletedIDs := make(chan string, 5)
	repo := &mocks.MockRoomRepository{
		ListExpiredFn: func(_ context.Context, _ time.Time) ([]*room.Room, error) {
			return []*room.Room{r}, nil
		},
		FindByIDFn: func(_ context.Context, id string) (*room.Room, error) {
			return r, nil
		},
		DeleteFn: func(_ context.Context, id string) error {
			deletedIDs <- id
			return nil
		},
		SaveFn: func(_ context.Context, _ *room.Room) error { return nil },
	}

	cfg := roomsvc.ServiceConfig{
		GracePeriod:         1 * time.Millisecond,
		RoomTTL:             10 * time.Minute,
		SweepInterval:       5 * time.Millisecond,
		MaxShortCodeRetries: 5,
	}
	svc := newServiceWithConfig(t, cfg, repo, &mocks.MockWebRTCPeer{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc.StartExpirationSweep(ctx)

	// Wait for at least one sweep cycle
	select {
	case id := <-deletedIDs:
		if id != "expired-room" {
			t.Errorf("expected expired-room to be deleted, got %q", id)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("sweep did not delete expired room within timeout")
	}
}

func TestService_StartExpirationSweep_StopsOnContextCancel(t *testing.T) {
	repo := &mocks.MockRoomRepository{
		ListExpiredFn: func(_ context.Context, _ time.Time) ([]*room.Room, error) {
			return nil, nil
		},
	}
	cfg := roomsvc.ServiceConfig{
		GracePeriod:         1 * time.Millisecond,
		RoomTTL:             10 * time.Minute,
		SweepInterval:       5 * time.Millisecond,
		MaxShortCodeRetries: 5,
	}
	svc := newServiceWithConfig(t, cfg, repo, &mocks.MockWebRTCPeer{})

	ctx, cancel := context.WithCancel(context.Background())
	svc.StartExpirationSweep(ctx)

	// Cancel immediately — goroutine must exit cleanly (no race/panic)
	cancel()
	time.Sleep(20 * time.Millisecond)
	// If we get here without data race, the test passes
}
