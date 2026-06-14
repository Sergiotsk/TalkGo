package driven

import "context"

// TextToSpeech defines the contract for text-to-speech synthesis services.
// Synthesize converts text into raw PCM16 LE audio at 24 kHz mono.
// The returned channel is closed when synthesis is complete or ctx is cancelled.
type TextToSpeech interface {
	Synthesize(ctx context.Context, text, lang string) (<-chan []byte, error)
}
