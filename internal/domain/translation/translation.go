package translation

import "time"

// Chunk represents a segment of audio or text being processed in the pipeline.
type Chunk struct {
	ID         string
	SessionID  string
	Payload    []byte
	SourceLang string
	TargetLang string
	Timestamp  time.Time
}

// NewChunk creates a new translation chunk with source and target language codes.
func NewChunk(id, sessionID string, payload []byte, sourceLang, targetLang string) *Chunk {
	return &Chunk{
		ID:         id,
		SessionID:  sessionID,
		Payload:    payload,
		SourceLang: sourceLang,
		TargetLang: targetLang,
		Timestamp:  time.Now(),
	}
}
