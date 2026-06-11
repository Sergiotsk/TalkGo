package driving

import (
	"context"
	"errors"

	"github.com/Sergiotsk/TalkGo/internal/domain/room"
)

// ErrRoomNotFound is returned when a room ID does not exist.
var ErrRoomNotFound = errors.New("room not found")

// CreateRoomResult holds the result of a successful CreateRoom call.
type CreateRoomResult struct {
	Room *room.Room
}

// RoomManager defines the driving port to manage TalkGo rooms.
type RoomManager interface {
	// CreateRoom creates a new TalkGo room with the specified language codes (ISO 639-1).
	// Returns CreateRoomResult on success (includes Room with ID and ShortCode).
	CreateRoom(ctx context.Context, sourceLang, targetLang string) (CreateRoomResult, error)

	// DeleteRoom closes and destroys an existing room, releasing all associated resources.
	// Returns ErrRoomNotFound if the roomID does not exist.
	DeleteRoom(ctx context.Context, roomID string) error

	// JoinRoom adds a user to an existing room and creates a new Session.
	// lang must be a non-empty ISO 639-1 code matching the room's SourceLang or TargetLang.
	// Returns the new session ID on success.
	// Propagates ErrRoomFull, ErrRoomClosed, ErrAlreadyInRoom from the domain.
	// Returns ErrRoomNotFound if the roomID does not exist.
	// Returns ErrMissingLang if lang is empty; ErrLangNotSupported if lang is not a room language.
	JoinRoom(ctx context.Context, roomID, userID, lang string) (string, error)

	// LeaveRoom disconnects a user from a room and cleans up their session.
	// Propagates ErrNotInRoom from the domain.
	// Returns ErrRoomNotFound if the roomID does not exist.
	LeaveRoom(ctx context.Context, roomID, userID string) error

	// RoomExists reports whether the given roomID exists and is active.
	// Returns ErrRoomNotFound if the room does not exist.
	RoomExists(ctx context.Context, roomID string) error

	// FindByShortCode looks up a room by its 6-char short code (case-insensitive).
	// Returns ErrRoomNotFound if no matching room exists.
	FindByShortCode(ctx context.Context, code string) (*room.Room, error)

	// UpdateLastActivity refreshes the LastActivity timestamp for the given room.
	// Returns ErrRoomNotFound if the room does not exist.
	UpdateLastActivity(ctx context.Context, roomID string) error
}
