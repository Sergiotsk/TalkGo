package translator

import (
	"context"
	"log/slog"

	"github.com/Sergiotsk/TalkGo/internal/adapters/tts"
	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
)

// PipelineTranslatorConfig holds all credentials needed for the three-stage pipeline.
type PipelineTranslatorConfig struct {
	APIKey string // shared across all OpenAI services
}

// PipelineTranslator implements driven.Translator by orchestrating:
//
//	STT (gpt-realtime-whisper) → Translate (gpt-4o) → TTS (gpt-4o-mini-tts)
//
// The driven.Translator interface is preserved — pipeline.go requires no changes.
type PipelineTranslator struct {
	stt       *WhisperSTT
	translate *TextTranslator
	ttsClient *tts.OpenAITTS
}

// NewPipelineTranslator creates a PipelineTranslator with the given API key.
func NewPipelineTranslator(cfg PipelineTranslatorConfig) *PipelineTranslator {
	return &PipelineTranslator{
		stt:       NewWhisperSTT(WhisperSTTConfig{APIKey: cfg.APIKey}),
		translate: NewTextTranslator(TextTranslatorConfig{APIKey: cfg.APIKey}),
		ttsClient: tts.NewOpenAITTS(tts.Config{APIKey: cfg.APIKey}),
	}
}

// TranslateStream implements driven.Translator.
// It starts the STT WebSocket, then for each final transcript it translates
// the text and synthesizes audio, forwarding both to the returned channels.
func (p *PipelineTranslator) TranslateStream(
	ctx context.Context,
	audioIn <-chan []byte,
	sourceLang, targetLang string,
) (driven.TranslateResult, error) {
	transcriptCh := make(chan string, 4)
	audioCh := make(chan []byte, 8)

	// Start STT — returns a channel of final transcript strings.
	sttCh, err := p.stt.Transcribe(ctx, audioIn, sourceLang)
	if err != nil {
		return driven.TranslateResult{}, err
	}

	// Orchestrator goroutine: translate each transcript, synthesize audio.
	go func() {
		defer close(transcriptCh)
		defer close(audioCh)

		for {
			select {
			case <-ctx.Done():
				return
			case original, ok := <-sttCh:
				if !ok {
					return
				}
				if original == "" {
					continue
				}

				// Stage 2: Translate text.
				translated, err := p.translate.Translate(ctx, original, sourceLang, targetLang)
				if err != nil {
					slog.Warn("pipeline_translate_error", "err", err, "source", original)
					continue
				}
				slog.Info("pipeline_translated", "src", sourceLang, "dst", targetLang, "original", original, "translated", translated)

				// Forward transcript to the target session (displayed on screen).
				select {
				case transcriptCh <- translated:
				default:
				}

				// Stage 3: Synthesize audio.
				pcmCh, err := p.ttsClient.Synthesize(ctx, translated, targetLang)
				if err != nil {
					slog.Warn("pipeline_tts_error", "err", err, "text", translated)
					continue
				}
				for pcm := range pcmCh {
					select {
					case audioCh <- pcm:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return driven.TranslateResult{Audio: audioCh, Transcript: transcriptCh}, nil
}

// Compile-time check.
var _ driven.Translator = (*PipelineTranslator)(nil)
