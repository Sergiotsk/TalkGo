package session

import (
	"time"
)

// Session represents an active WebRTC session for a participant in a room.
type Session struct {
	ID        string
	RoomID    string
	UserID    string
	JoinedAt  time.Time
	Active    bool
}

// NewSession creates and initializes a new Session.
func NewSession(id, roomID, userID string) *Session {
	return &Session{
		ID:       id,
		RoomID:   roomID,
		UserID:   userID,
		JoinedAt: time.Now(),
		Active:   true,
	}
}
