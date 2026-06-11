package translation

import (
	"bytes"
	"testing"
)

func TestNewChunk_WithLanguages(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		sessionID  string
		payload    []byte
		sourceLang string
		targetLang string
	}{
		{
			name:       "spanish to english",
			id:         "chunk-1",
			sessionID:  "sess-1",
			payload:    []byte("hola"),
			sourceLang: "es",
			targetLang: "en",
		},
		{
			name:       "english to spanish",
			id:         "chunk-2",
			sessionID:  "sess-2",
			payload:    []byte("hello"),
			sourceLang: "en",
			targetLang: "es",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunk := NewChunk(tt.id, tt.sessionID, tt.payload, tt.sourceLang, tt.targetLang)
			if chunk.SourceLang != tt.sourceLang {
				t.Errorf("NewChunk() SourceLang = %q, want %q", chunk.SourceLang, tt.sourceLang)
			}
			if chunk.TargetLang != tt.targetLang {
				t.Errorf("NewChunk() TargetLang = %q, want %q", chunk.TargetLang, tt.targetLang)
			}
			if !bytes.Equal(chunk.Payload, tt.payload) {
				t.Errorf("NewChunk() Payload = %q, want %q", chunk.Payload, tt.payload)
			}
		})
	}
}
