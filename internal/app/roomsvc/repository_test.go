package roomsvc_test

import (
	"context"
	"errors"
	"sync"
	"testing"

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
