package driven

import (
	"context"
	"time"

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

	// FindByShortCode retrieves a room by its short code (case-insensitive, normalized to uppercase).
	// Returns ErrRoomNotFound (from driving package) if not found.
	FindByShortCode(ctx context.Context, code string) (*room.Room, error)

	// UpdateLastActivity refreshes the LastActivity timestamp for the given room.
	// Returns ErrRoomNotFound (from driving package) if the room does not exist.
	UpdateLastActivity(ctx context.Context, roomID string) error

	// ListExpired returns all rooms whose LastActivity is before the given time.
	// Used by the expiration sweep goroutine.
	ListExpired(ctx context.Context, before time.Time) ([]*room.Room, error)
}
