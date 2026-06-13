package roomsvc_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Sergiotsk/TalkGo/internal/app/roomsvc"
	"github.com/Sergiotsk/TalkGo/internal/domain/room"
	"github.com/Sergiotsk/TalkGo/internal/ports/driving"
)

func TestInMemoryRoomRepository_Save_FindByID(t *testing.T) {
	repo := roomsvc.NewInMemoryRoomRepository()
	r, _ := room.NewRoom("room-1", "es", "en")

	if err := repo.Save(context.Background(), r); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.FindByID(context.Background(), "room-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.ID != "room-1" {
		t.Errorf("got ID %q, want %q", got.ID, "room-1")
	}
}

func TestInMemoryRoomRepository_FindByID_NotFound(t *testing.T) {
	repo := roomsvc.NewInMemoryRoomRepository()

	_, err := repo.FindByID(context.Background(), "nonexistent")

	if !errors.Is(err, driving.ErrRoomNotFound) {
		t.Errorf("expected ErrRoomNotFound, got %v", err)
	}
}

func TestInMemoryRoomRepository_Delete(t *testing.T) {
	repo := roomsvc.NewInMemoryRoomRepository()
	r, _ := room.NewRoom("room-1", "es", "en")
	_ = repo.Save(context.Background(), r)

	if err := repo.Delete(context.Background(), "room-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.FindByID(context.Background(), "room-1")
	if !errors.Is(err, driving.ErrRoomNotFound) {
		t.Errorf("expected ErrRoomNotFound after delete, got %v", err)
	}
}

func TestInMemoryRoomRepository_Delete_Idempotent(t *testing.T) {
	repo := roomsvc.NewInMemoryRoomRepository()

	// Deleting a non-existent room must not return an error
	if err := repo.Delete(context.Background(), "nonexistent"); err != nil {
		t.Errorf("Delete of non-existent room should be idempotent, got: %v", err)
	}
}

func TestInMemoryRoomRepository_ListActive(t *testing.T) {
	repo := roomsvc.NewInMemoryRoomRepository()
	r1, _ := room.NewRoom("room-1", "es", "en")
	r2, _ := room.NewRoom("room-2", "fr", "de")
	r3, _ := room.NewRoom("room-3", "pt", "en")
	r3.Close() // inactive

	_ = repo.Save(context.Background(), r1)
	_ = repo.Save(context.Background(), r2)
	_ = repo.Save(context.Background(), r3)

	active, err := repo.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("expected 2 active rooms, got %d", len(active))
	}
}

func TestInMemoryRoomRepository_ConcurrentSave(t *testing.T) {
	repo := roomsvc.NewInMemoryRoomRepository()
	const goroutines = 10

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, _ := room.NewRoom("shared-room", "es", "en")
			_ = repo.Save(context.Background(), r)
		}()
	}
	wg.Wait()

	// After concurrent saves the room must be retrievable without error
	if _, err := repo.FindByID(context.Background(), "shared-room"); err != nil {
		t.Errorf("expected room after concurrent saves, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// FindByShortCode
// ---------------------------------------------------------------------------

func TestInMemoryRoomRepository_FindByShortCode_HappyPath(t *testing.T) {
	repo := roomsvc.NewInMemoryRoomRepository()
	r, _ := room.NewRoom("room-sc-1", "es", "en")
	r.ShortCode = "ABCD12"
	_ = repo.Save(context.Background(), r)

	got, err := repo.FindByShortCode(context.Background(), "ABCD12")
	if err != nil {
		t.Fatalf("FindByShortCode: %v", err)
	}
	if got.ID != "room-sc-1" {
		t.Errorf("got ID %q, want %q", got.ID, "room-sc-1")
	}
}

func TestInMemoryRoomRepository_FindByShortCode_CaseInsensitive(t *testing.T) {
	repo := roomsvc.NewInMemoryRoomRepository()
	r, _ := room.NewRoom("room-sc-2", "es", "en")
	r.ShortCode = "XYZ789"
	_ = repo.Save(context.Background(), r)

	got, err := repo.FindByShortCode(context.Background(), "xyz789")
	if err != nil {
		t.Fatalf("FindByShortCode lowercase: %v", err)
	}
	if got.ID != "room-sc-2" {
		t.Errorf("got ID %q, want %q", got.ID, "room-sc-2")
	}
}

func TestInMemoryRoomRepository_FindByShortCode_NotFound(t *testing.T) {
	repo := roomsvc.NewInMemoryRoomRepository()

	_, err := repo.FindByShortCode(context.Background(), "ZZZZZZ")
	if !errors.Is(err, driving.ErrRoomNotFound) {
		t.Errorf("expected ErrRoomNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// UpdateLastActivity
// ---------------------------------------------------------------------------

func TestInMemoryRoomRepository_UpdateLastActivity_HappyPath(t *testing.T) {
	repo := roomsvc.NewInMemoryRoomRepository()
	r, _ := room.NewRoom("room-la-1", "es", "en")
	_ = repo.Save(context.Background(), r)

	before := time.Now()
	if err := repo.UpdateLastActivity(context.Background(), "room-la-1"); err != nil {
		t.Fatalf("UpdateLastActivity: %v", err)
	}
	after := time.Now()

	got, _ := repo.FindByID(context.Background(), "room-la-1")
	if got.LastActivity.Before(before) || got.LastActivity.After(after) {
		t.Errorf("LastActivity %v not in [%v, %v]", got.LastActivity, before, after)
	}
}

func TestInMemoryRoomRepository_UpdateLastActivity_NotFound(t *testing.T) {
	repo := roomsvc.NewInMemoryRoomRepository()

	err := repo.UpdateLastActivity(context.Background(), "nonexistent")
	if !errors.Is(err, driving.ErrRoomNotFound) {
		t.Errorf("expected ErrRoomNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListExpired
// ---------------------------------------------------------------------------

func TestInMemoryRoomRepository_ListExpired_ReturnsExpired(t *testing.T) {
	repo := roomsvc.NewInMemoryRoomRepository()
	r1, _ := room.NewRoom("room-exp-1", "es", "en")
	r1.LastActivity = time.Now().Add(-20 * time.Minute)
	r2, _ := room.NewRoom("room-exp-2", "es", "en")
	r2.LastActivity = time.Now().Add(-5 * time.Minute)
	_ = repo.Save(context.Background(), r1)
	_ = repo.Save(context.Background(), r2)

	// Rooms with LastActivity before 10 minutes ago
	expired, err := repo.ListExpired(context.Background(), time.Now().Add(-10*time.Minute))
	if err != nil {
		t.Fatalf("ListExpired: %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired room, got %d", len(expired))
	}
	if expired[0].ID != "room-exp-1" {
		t.Errorf("expected room-exp-1, got %q", expired[0].ID)
	}
}

func TestInMemoryRoomRepository_ListExpired_ZeroLastActivity_NotExpired(t *testing.T) {
	repo := roomsvc.NewInMemoryRoomRepository()
	r, _ := room.NewRoom("room-zero", "es", "en")
	// LastActivity is zero — must NOT be considered expired (room never had activity)
	_ = repo.Save(context.Background(), r)

	expired, err := repo.ListExpired(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("ListExpired: %v", err)
	}
	for _, e := range expired {
		if e.ID == "room-zero" {
			t.Error("room with zero LastActivity should not be returned by ListExpired")
		}
	}
}

func TestInMemoryRoomRepository_ListExpired_Empty(t *testing.T) {
	repo := roomsvc.NewInMemoryRoomRepository()

	expired, err := repo.ListExpired(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("ListExpired on empty repo: %v", err)
	}
	if len(expired) != 0 {
		t.Errorf("expected 0 expired rooms, got %d", len(expired))
	}
}

// ---------------------------------------------------------------------------
// CRIT-01: ListExpired returns []*room.Room (pointer, not value)
// ---------------------------------------------------------------------------

// TestListExpired_ReturnsPointers verifies that ListExpired returns a slice of
// *room.Room pointers, NOT []room.Room values. The Room struct contains a
// sync.Mutex field which triggers copylock warnings if passed by value.
// This test uses type assertion at compile time — it won't compile if the
// return type is []room.Room (value).
func TestListExpired_ReturnsPointers(t *testing.T) {
	repo := roomsvc.NewInMemoryRoomRepository()

	expired, err := repo.ListExpired(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("ListExpired: %v", err)
	}

	// CRIT-01 verification: type-assert that each element is *room.Room
	// (not room.Room value). The range variable r must be assignable to *room.Room.
	for _, r := range expired {
		if r == nil {
			continue // nil pointers are fine — empty repo returns nil slice
		}
		// Compile-time check: r must be *room.Room (pointer), not room.Room (value).
		// If someone changes ListExpired to return []room.Room, the field access
		// below still works (Go promotes fields), but `r` would be a copy that
		// includes a copy of sync.Mutex — caught by `go vet` as copylock.
		_ = r.ID
		_ = r.ShortCode
	}

	// Explicit type assertion at the interface level: verify that the repo
	// method satisfies the expected signature. This is a compile-time check.
	type listExpiredFunc func(context.Context, time.Time) ([]*room.Room, error)
	var _ listExpiredFunc = repo.ListExpired
	_ = expired
}
