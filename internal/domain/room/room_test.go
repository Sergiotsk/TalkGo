package room

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestNewRoom(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		sourceLang string
		targetLang string
		wantErr    error
	}{
		{
			name:       "valid room",
			id:         "room-1",
			sourceLang: "es",
			targetLang: "en",
			wantErr:    nil,
		},
		{
			name:       "invalid source language code length",
			id:         "room-2",
			sourceLang: "spa",
			targetLang: "en",
			wantErr:    ErrInvalidLanguageCode,
		},
		{
			name:       "invalid target language code length",
			id:         "room-3",
			sourceLang: "es",
			targetLang: "eng",
			wantErr:    ErrInvalidLanguageCode,
		},
		{
			name:       "empty source language",
			id:         "room-4",
			sourceLang: "",
			targetLang: "en",
			wantErr:    ErrInvalidLanguageCode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewRoom(tt.id, tt.sourceLang, tt.targetLang)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if r.ID != tt.id {
				t.Errorf("got ID %q, want %q", r.ID, tt.id)
			}
			if r.SourceLang != tt.sourceLang {
				t.Errorf("got SourceLang %q, want %q", r.SourceLang, tt.sourceLang)
			}
			if r.TargetLang != tt.targetLang {
				t.Errorf("got TargetLang %q, want %q", r.TargetLang, tt.targetLang)
			}
			if !r.Active {
				t.Errorf("expected room to be active")
			}
			if len(r.Participants) != 0 {
				t.Errorf("expected 0 participants, got %d", len(r.Participants))
			}
			if r.Capacity != 2 {
				t.Errorf("expected Capacity=2, got %d", r.Capacity)
			}
		})
	}
}

func TestRoomJoin(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Room)
		userID  string
		wantErr error
	}{
		{
			name:    "first participant joins empty room",
			setup:   func(r *Room) {},
			userID:  "user-1",
			wantErr: nil,
		},
		{
			name: "second participant joins room with one",
			setup: func(r *Room) {
				_ = r.Join("user-1")
			},
			userID:  "user-2",
			wantErr: nil,
		},
		{
			name: "room full returns ErrRoomFull",
			setup: func(r *Room) {
				_ = r.Join("user-1")
				_ = r.Join("user-2")
			},
			userID:  "user-3",
			wantErr: ErrRoomFull,
		},
		{
			name: "duplicate user returns ErrAlreadyInRoom",
			setup: func(r *Room) {
				_ = r.Join("user-1")
			},
			userID:  "user-1",
			wantErr: ErrAlreadyInRoom,
		},
		{
			name: "closed room returns ErrRoomClosed",
			setup: func(r *Room) {
				r.Close()
			},
			userID:  "user-1",
			wantErr: ErrRoomClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewRoom("room-1", "es", "en")
			if err != nil {
				t.Fatalf("unexpected error creating room: %v", err)
			}

			tt.setup(r)
			err = r.Join(tt.userID)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Join() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Join() unexpected error: %v", err)
			}
			if _, ok := r.Participants[tt.userID]; !ok {
				t.Errorf("expected %q in Participants after join", tt.userID)
			}
		})
	}
}

func TestRoomJoinConcurrent(t *testing.T) {
	r, err := NewRoom("room-1", "es", "en")
	if err != nil {
		t.Fatalf("unexpected error creating room: %v", err)
	}

	const goroutines = 5
	var wg sync.WaitGroup
	results := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = r.Join(fmt.Sprintf("user-%d", idx))
		}(i)
	}

	wg.Wait()

	successCount := 0
	for _, res := range results {
		if res == nil {
			successCount++
		}
	}
	// Capacity is 2: exactly 2 of the 5 concurrent joins should succeed
	if successCount != 2 {
		t.Errorf("concurrent joins: got %d successes, want 2", successCount)
	}
	if len(r.Participants) != 2 {
		t.Errorf("expected 2 participants after concurrent joins, got %d", len(r.Participants))
	}
}

func TestRoomLeave(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(*Room)
		userID        string
		wantErr       error
		wantRemaining int
	}{
		{
			name: "participant leaves successfully",
			setup: func(r *Room) {
				_ = r.Join("user-1")
			},
			userID:        "user-1",
			wantErr:       nil,
			wantRemaining: 0,
		},
		{
			name:          "unknown user returns ErrNotInRoom",
			setup:         func(r *Room) {},
			userID:        "user-1",
			wantErr:       ErrNotInRoom,
			wantRemaining: 0,
		},
		{
			name: "correct participant leaves room with two",
			setup: func(r *Room) {
				_ = r.Join("user-1")
				_ = r.Join("user-2")
			},
			userID:        "user-1",
			wantErr:       nil,
			wantRemaining: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewRoom("room-1", "es", "en")
			if err != nil {
				t.Fatalf("unexpected error creating room: %v", err)
			}

			tt.setup(r)
			err = r.Leave(tt.userID)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Leave() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Leave() unexpected error: %v", err)
			}
			if _, ok := r.Participants[tt.userID]; ok {
				t.Errorf("expected %q to be removed from Participants", tt.userID)
			}
			if len(r.Participants) != tt.wantRemaining {
				t.Errorf("expected %d remaining participants, got %d", tt.wantRemaining, len(r.Participants))
			}
		})
	}
}

func TestRoomClose(t *testing.T) {
	r, err := NewRoom("room-1", "es", "en")
	if err != nil {
		t.Fatalf("unexpected error creating room: %v", err)
	}

	_ = r.Join("user-1")
	r.Close()

	if r.Active {
		t.Error("expected room to be inactive after Close()")
	}
	if len(r.Participants) != 0 {
		t.Errorf("expected 0 participants after Close(), got %d", len(r.Participants))
	}

	// Joining a closed room must return ErrRoomClosed
	if err := r.Join("user-2"); !errors.Is(err, ErrRoomClosed) {
		t.Errorf("expected ErrRoomClosed after Close(), got %v", err)
	}
}

func TestRoomIsFull(t *testing.T) {
	r, _ := NewRoom("room-1", "es", "en")

	if r.IsFull() {
		t.Error("new room should not be full")
	}

	_ = r.Join("user-1")
	if r.IsFull() {
		t.Error("room with 1/2 participants should not be full")
	}

	_ = r.Join("user-2")
	if !r.IsFull() {
		t.Error("room with 2/2 participants should be full")
	}
}
