package driven

import "context"

// Translator defines the contract for real-time translation services.
// Implementations: OpenAI Realtime (primary), Whisper+GPT+ElevenLabs pipeline (fallback).
type Translator interface {
	// TranslateStream receives audio chunks and returns translated audio chunks.
	// sourceLang and targetLang are ISO 639-1 codes.
	TranslateStream(ctx context.Context, sourceLang, targetLang string, audioIn <-chan []byte) (<-chan []byte, error)
}
