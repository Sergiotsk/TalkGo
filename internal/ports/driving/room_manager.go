package driving

import "context"

// RoomManager defines the driving port to manage TalkGo rooms.
type RoomManager interface {
	// CreateRoom creates a new TalkGo room with the specified languages.
	CreateRoom(ctx context.Context, sourceLang, targetLang string) (string, error)
	// DeleteRoom closes and destroys an existing room.
	DeleteRoom(ctx context.Context, roomID string) error
}
