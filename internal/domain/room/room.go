package room

import (
	"errors"
	"sync"
	"time"
)

var (
	// ErrInvalidLanguageCode is returned when a language code is not a valid ISO 639-1 2-character code.
	ErrInvalidLanguageCode = errors.New("invalid language code: must be ISO 639-1 (2 characters)")
	// ErrRoomFull is returned when a room has reached its participant capacity.
	ErrRoomFull = errors.New("room is full")
	// ErrRoomClosed is returned when an operation is attempted on a closed room.
	ErrRoomClosed = errors.New("room is closed")
	// ErrAlreadyInRoom is returned when a user tries to join a room they are already in.
	ErrAlreadyInRoom = errors.New("user is already in this room")
	// ErrNotInRoom is returned when a user tries to leave a room they are not in.
	ErrNotInRoom = errors.New("user is not in this room")
)

const defaultCapacity = 2

// Room represents an active translation room between two languages.
type Room struct {
	ID           string
	SourceLang   string
	TargetLang   string
	CreatedAt    time.Time
	Active       bool
	Participants map[string]struct{}
	Capacity     int
	mu           sync.Mutex
}

// NewRoom creates and initializes a new Room with language validation.
func NewRoom(id, sourceLang, targetLang string) (*Room, error) {
	if len(sourceLang) != 2 || len(targetLang) != 2 {
		return nil, ErrInvalidLanguageCode
	}
	return &Room{
		ID:           id,
		SourceLang:   sourceLang,
		TargetLang:   targetLang,
		CreatedAt:    time.Now(),
		Active:       true,
		Participants: make(map[string]struct{}),
		Capacity:     defaultCapacity,
	}, nil
}

// Join adds a participant to the room.
// Returns ErrRoomClosed if the room is inactive, ErrAlreadyInRoom if the user is already
// a participant, or ErrRoomFull if the room has reached its capacity.
func (r *Room) Join(userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.Active {
		return ErrRoomClosed
	}
	if _, ok := r.Participants[userID]; ok {
		return ErrAlreadyInRoom
	}
	if len(r.Participants) >= r.Capacity {
		return ErrRoomFull
	}
	r.Participants[userID] = struct{}{}
	return nil
}

// Leave removes a participant from the room.
// Returns ErrNotInRoom if the user is not a participant.
func (r *Room) Leave(userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.Participants[userID]; !ok {
		return ErrNotInRoom
	}
	delete(r.Participants, userID)
	return nil
}

// IsFull reports whether the room has reached its participant capacity.
func (r *Room) IsFull() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.Participants) >= r.Capacity
}

// Close sets the room as inactive and removes all participants.
// Once closed, a room cannot be reopened.
func (r *Room) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Active = false
	r.Participants = make(map[string]struct{})
}
