package driven

import (
	"context"

	"github.com/Sergiotsk/TalkGo/internal/domain/room"
)

// RoomRepository defines the driven port for room persistence.
// The Sprint 1 implementation is an in-memory store in internal/app/roomsvc/.
type RoomRepository interface {
	// Save persists a room. Overwrites if the room ID already exists.
	Save(ctx context.Context, r *room.Room) error

	// FindByID retrieves a room by its ID.
	// Returns ErrRoomNotFound (from driving package) if not found.
	FindByID(ctx context.Context, roomID string) (*room.Room, error)

	// Delete removes a room from the store.
	// Idempotent: returns nil if the room does not exist.
	Delete(ctx context.Context, roomID string) error

	// ListActive returns all rooms where Active == true.
	ListActive(ctx context.Context) ([]*room.Room, error)
}
