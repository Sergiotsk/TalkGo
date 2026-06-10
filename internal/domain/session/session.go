package session

import (
	"errors"
	"time"
)

// ErrInvalidTransition is returned when a state transition is not allowed.
var ErrInvalidTransition = errors.New("invalid session state transition")

// State represents the lifecycle state of a Session.
type State int

const (
	// StateConnecting is the initial state: session created, WebRTC handshake not yet complete.
	StateConnecting State = iota
	// StateActive means the WebRTC handshake is complete and media is flowing.
	StateActive
	// StateDisconnected means the session has ended (graceful or error).
	StateDisconnected
)

// Session represents an active WebRTC session for a participant in a room.
type Session struct {
	ID       string
	RoomID   string
	UserID   string
	JoinedAt time.Time
	State    State
}

// NewSession creates and initializes a new Session in StateConnecting.
func NewSession(id, roomID, userID string) *Session {
	return &Session{
		ID:       id,
		RoomID:   roomID,
		UserID:   userID,
		JoinedAt: time.Now(),
		State:    StateConnecting,
	}
}

// Activate transitions the session from StateConnecting to StateActive.
// Returns ErrInvalidTransition if the session is not in StateConnecting.
func (s *Session) Activate() error {
	if s.State != StateConnecting {
		return ErrInvalidTransition
	}
	s.State = StateActive
	return nil
}

// Disconnect transitions the session to StateDisconnected.
// Safe to call from any state; calling it on an already-disconnected session is a no-op.
func (s *Session) Disconnect() error {
	s.State = StateDisconnected
	return nil
}

// IsActive reports whether the session is in StateActive.
func (s *Session) IsActive() bool {
	return s.State == StateActive
}
