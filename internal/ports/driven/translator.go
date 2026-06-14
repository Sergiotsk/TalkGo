package driven

import "context"

// TranslateResult holds both the translated audio channel and the transcript text channel.
// Transcript receives complete utterance strings as they are finalised by the translation service.
// The transcript channel is closed when the stream ends.
type TranslateResult struct {
	Audio      <-chan []byte
	Transcript <-chan string
}

// Translator defines the contract for real-time translation services.
// Implementations: OpenAI Realtime (primary), Whisper+GPT+ElevenLabs pipeline (fallback).
type Translator interface {
	// TranslateStream receives audio chunks and returns translated audio and transcript channels.
	// sourceLang and targetLang are ISO 639-1 codes.
	TranslateStream(ctx context.Context, audioIn <-chan []byte, sourceLang, targetLang string) (TranslateResult, error)
}
