package roomsvc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Sergiotsk/TalkGo/internal/domain/room"
	"github.com/Sergiotsk/TalkGo/internal/ports/driving"
)

// InMemoryRoomRepository is a thread-safe in-memory implementation of driven.RoomRepository.
// It is suitable for Sprint 1 MVP; all state is lost on process restart.
type InMemoryRoomRepository struct {
	mu    sync.RWMutex
	rooms map[string]*room.Room
}

// NewInMemoryRoomRepository creates an empty InMemoryRoomRepository.
func NewInMemoryRoomRepository() *InMemoryRoomRepository {
	return &InMemoryRoomRepository{
		rooms: make(map[string]*room.Room),
	}
}

// Save persists a room, overwriting any existing entry with the same ID.
func (r *InMemoryRoomRepository) Save(_ context.Context, rm *room.Room) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rooms[rm.ID] = rm
	return nil
}

// FindByID retrieves a room by its ID.
// Returns driving.ErrRoomNotFound wrapped with context if not found.
func (r *InMemoryRoomRepository) FindByID(_ context.Context, roomID string) (*room.Room, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rm, ok := r.rooms[roomID]
	if !ok {
		return nil, fmt.Errorf("roomsvc.FindByID: %w", driving.ErrRoomNotFound)
	}
	return rm, nil
}

// Delete removes a room from the store. Idempotent: returns nil if room not found.
func (r *InMemoryRoomRepository) Delete(_ context.Context, roomID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rooms, roomID)
	return nil
}

// ListActive returns all rooms where Active == true.
func (r *InMemoryRoomRepository) ListActive(_ context.Context) ([]*room.Room, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var active []*room.Room
	for _, rm := range r.rooms {
		if rm.Active {
			active = append(active, rm)
		}
	}
	return active, nil
}

// FindByShortCode retrieves a room by its short code (case-insensitive).
func (r *InMemoryRoomRepository) FindByShortCode(_ context.Context, code string) (*room.Room, error) {
	normalized := strings.ToUpper(code)
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, rm := range r.rooms {
		if rm.ShortCode == normalized {
			return rm, nil
		}
	}
	return nil, fmt.Errorf("roomsvc.FindByShortCode: %w", driving.ErrRoomNotFound)
}

// UpdateLastActivity refreshes the LastActivity timestamp for the given room.
func (r *InMemoryRoomRepository) UpdateLastActivity(_ context.Context, roomID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rm, ok := r.rooms[roomID]
	if !ok {
		return fmt.Errorf("roomsvc.UpdateLastActivity: %w", driving.ErrRoomNotFound)
	}
	rm.LastActivity = time.Now()
	return nil
}

// ListExpired returns all rooms whose LastActivity is non-zero and before the given time.
func (r *InMemoryRoomRepository) ListExpired(_ context.Context, before time.Time) ([]*room.Room, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var expired []*room.Room
	for _, rm := range r.rooms {
		if !rm.LastActivity.IsZero() && rm.LastActivity.Before(before) {
			expired = append(expired, rm)
		}
	}
	return expired, nil
}
