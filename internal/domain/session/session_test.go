package session

import (
	"errors"
	"testing"
)

func TestNewSession(t *testing.T) {
	s := NewSession("s1", "room-1", "user-1", "")

	if s.ID != "s1" {
		t.Errorf("got ID %q, want %q", s.ID, "s1")
	}
	if s.RoomID != "room-1" {
		t.Errorf("got RoomID %q, want %q", s.RoomID, "room-1")
	}
	if s.UserID != "user-1" {
		t.Errorf("got UserID %q, want %q", s.UserID, "user-1")
	}
	if s.State != StateConnecting {
		t.Errorf("expected StateConnecting on creation, got %v", s.State)
	}
	if s.IsActive() {
		t.Error("new session should not be active")
	}
}

func TestSessionActivate(t *testing.T) {
	tests := []struct {
		name       string
		startState State
		wantErr    error
		wantState  State
	}{
		{
			name:       "connecting to active",
			startState: StateConnecting,
			wantErr:    nil,
			wantState:  StateActive,
		},
		{
			name:       "already active returns ErrInvalidTransition",
			startState: StateActive,
			wantErr:    ErrInvalidTransition,
			wantState:  StateActive,
		},
		{
			name:       "disconnected returns ErrInvalidTransition",
			startState: StateDisconnected,
			wantErr:    ErrInvalidTransition,
			wantState:  StateDisconnected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSession("s1", "room-1", "user-1", "")
			s.State = tt.startState

			err := s.Activate()

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Activate() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("Activate() unexpected error: %v", err)
			}

			if s.State != tt.wantState {
				t.Errorf("State = %v, want %v", s.State, tt.wantState)
			}
		})
	}
}

func TestSessionIsActive(t *testing.T) {
	tests := []struct {
		name       string
		startState State
		want       bool
	}{
		{"connecting is not active", StateConnecting, false},
		{"active is active", StateActive, true},
		{"disconnected is not active", StateDisconnected, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSession("s1", "room-1", "user-1", "")
			s.State = tt.startState

			if got := s.IsActive(); got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewSession_WithLang(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		roomID   string
		userID   string
		lang     string
		wantLang string
	}{
		{
			name:     "stores lang correctly",
			id:       "sess-1",
			roomID:   "room-1",
			userID:   "user-1",
			lang:     "es",
			wantLang: "es",
		},
		{
			name:     "stores english lang",
			id:       "sess-2",
			roomID:   "room-1",
			userID:   "user-2",
			lang:     "en",
			wantLang: "en",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := NewSession(tt.id, tt.roomID, tt.userID, tt.lang)
			if sess.Lang != tt.wantLang {
				t.Errorf("NewSession() Lang = %q, want %q", sess.Lang, tt.wantLang)
			}
		})
	}
}

func TestSessionDisconnect(t *testing.T) {
	tests := []struct {
		name       string
		startState State
		wantErr    error
	}{
		{
			name:       "connecting to disconnected",
			startState: StateConnecting,
			wantErr:    nil,
		},
		{
			name:       "active to disconnected",
			startState: StateActive,
			wantErr:    nil,
		},
		{
			name:       "already disconnected is idempotent",
			startState: StateDisconnected,
			wantErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSession("s1", "room-1", "user-1", "")
			s.State = tt.startState

			err := s.Disconnect()

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Disconnect() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("Disconnect() unexpected error: %v", err)
			}
			if s.State != StateDisconnected {
				t.Errorf("State = %v after Disconnect(), want StateDisconnected", s.State)
			}
		})
	}
}
