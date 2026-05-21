package translation

import "time"

// Chunk represents a segment of audio or text being processed in the pipeline.
type Chunk struct {
	ID        string
	SessionID string
	Payload   []byte
	Timestamp time.Time
}

// NewChunk creates a new translation chunk.
func NewChunk(id, sessionID string, payload []byte) *Chunk {
	return &Chunk{
		ID:        id,
		SessionID: sessionID,
		Payload:   payload,
		Timestamp: time.Now(),
	}
}
