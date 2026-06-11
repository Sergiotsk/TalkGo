// Package mocks provides hand-written test doubles for driven port interfaces.
package mocks

import (
	"context"
	"time"

	"github.com/Sergiotsk/TalkGo/internal/domain/room"
)

// MockRoomRepository is a test double for driven.RoomRepository.
// Configure behaviour by assigning the Fn fields before use.
type MockRoomRepository struct {
	SaveFn               func(ctx context.Context, r *room.Room) error
	FindByIDFn           func(ctx context.Context, roomID string) (*room.Room, error)
	DeleteFn             func(ctx context.Context, roomID string) error
	ListActiveFn         func(ctx context.Context) ([]*room.Room, error)
	FindByShortCodeFn    func(ctx context.Context, code string) (*room.Room, error)
	UpdateLastActivityFn func(ctx context.Context, roomID string) error
	ListExpiredFn        func(ctx context.Context, before time.Time) ([]*room.Room, error)

	SaveCalled               int
	FindByIDCalled           int
	DeleteCalled             int
	ListActiveCalled         int
	FindByShortCodeCalled    int
	UpdateLastActivityCalled int
	ListExpiredCalled        int
}

// Save implements driven.RoomRepository.
func (m *MockRoomRepository) Save(ctx context.Context, r *room.Room) error {
	m.SaveCalled++
	if m.SaveFn != nil {
		return m.SaveFn(ctx, r)
	}
	return nil
}

// FindByID implements driven.RoomRepository.
func (m *MockRoomRepository) FindByID(ctx context.Context, roomID string) (*room.Room, error) {
	m.FindByIDCalled++
	if m.FindByIDFn != nil {
		return m.FindByIDFn(ctx, roomID)
	}
	return nil, nil
}

// Delete implements driven.RoomRepository.
func (m *MockRoomRepository) Delete(ctx context.Context, roomID string) error {
	m.DeleteCalled++
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, roomID)
	}
	return nil
}

// ListActive implements driven.RoomRepository.
func (m *MockRoomRepository) ListActive(ctx context.Context) ([]*room.Room, error) {
	m.ListActiveCalled++
	if m.ListActiveFn != nil {
		return m.ListActiveFn(ctx)
	}
	return nil, nil
}

// FindByShortCode implements driven.RoomRepository.
func (m *MockRoomRepository) FindByShortCode(ctx context.Context, code string) (*room.Room, error) {
	m.FindByShortCodeCalled++
	if m.FindByShortCodeFn != nil {
		return m.FindByShortCodeFn(ctx, code)
	}
	return nil, nil
}

// UpdateLastActivity implements driven.RoomRepository.
func (m *MockRoomRepository) UpdateLastActivity(ctx context.Context, roomID string) error {
	m.UpdateLastActivityCalled++
	if m.UpdateLastActivityFn != nil {
		return m.UpdateLastActivityFn(ctx, roomID)
	}
	return nil
}

// ListExpired implements driven.RoomRepository.
func (m *MockRoomRepository) ListExpired(ctx context.Context, before time.Time) ([]*room.Room, error) {
	m.ListExpiredCalled++
	if m.ListExpiredFn != nil {
		return m.ListExpiredFn(ctx, before)
	}
	return nil, nil
}
