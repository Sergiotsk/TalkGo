package room

import (
	"errors"
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
		})
	}
}
