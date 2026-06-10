package driving

import (
	"context"
	"errors"
)

// ErrRoomNotFound is returned when a room ID does not exist.
var ErrRoomNotFound = errors.New("room not found")

// RoomManager defines the driving port to manage TalkGo rooms.
type RoomManager interface {
	// CreateRoom creates a new TalkGo room with the specified language codes (ISO 639-1).
	// Returns the new room ID on success.
	CreateRoom(ctx context.Context, sourceLang, targetLang string) (string, error)

	// DeleteRoom closes and destroys an existing room, releasing all associated resources.
	// Returns ErrRoomNotFound if the roomID does not exist.
	DeleteRoom(ctx context.Context, roomID string) error

	// JoinRoom adds a user to an existing room and creates a new Session.
	// Returns the new session ID on success.
	// Propagates ErrRoomFull, ErrRoomClosed, ErrAlreadyInRoom from the domain.
	// Returns ErrRoomNotFound if the roomID does not exist.
	JoinRoom(ctx context.Context, roomID, userID string) (string, error)

	// LeaveRoom disconnects a user from a room and cleans up their session.
	// Propagates ErrNotInRoom from the domain.
	// Returns ErrRoomNotFound if the roomID does not exist.
	LeaveRoom(ctx context.Context, roomID, userID string) error

	// RoomExists reports whether the given roomID exists and is active.
	// Returns ErrRoomNotFound if the room does not exist.
	RoomExists(ctx context.Context, roomID string) error
}
