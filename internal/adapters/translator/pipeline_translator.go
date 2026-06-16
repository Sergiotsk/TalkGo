package translator

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	lingua "github.com/pemistahl/lingua-go"

	"github.com/Sergiotsk/TalkGo/internal/adapters/tts"
	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
)

// PipelineTranslatorConfig holds all credentials needed for the three-stage pipeline.
type PipelineTranslatorConfig struct {
	APIKey string // shared across all OpenAI services
}

// PipelineTranslator implements driven.Translator by orchestrating:
//
//	STT (gpt-realtime-whisper) → Translate (gpt-4o-mini) → TTS (gpt-4o-mini-tts)
//
// Translations run concurrently (fan-out) and are drained in arrival order so
// audio playback remains coherent. See ADR-0005, ADR-0006, ADR-0007.
type PipelineTranslator struct {
	stt          *WhisperSTT
	translate    *TextTranslator
	ttsClient    *tts.OpenAITTS
	langDetector lingua.LanguageDetector
}

// NewPipelineTranslator creates a PipelineTranslator with the given API key.
func NewPipelineTranslator(cfg PipelineTranslatorConfig) *PipelineTranslator {
	detector := lingua.NewLanguageDetectorBuilder().
		FromAllLanguages().
		WithLowAccuracyMode().
		Build()

	return &PipelineTranslator{
		stt:          NewWhisperSTT(WhisperSTTConfig{APIKey: cfg.APIKey}),
		translate:    NewTextTranslator(TextTranslatorConfig{APIKey: cfg.APIKey}),
		ttsClient:    tts.NewOpenAITTS(tts.Config{APIKey: cfg.APIKey}),
		langDetector: detector,
	}
}

// translationResult carries an ordered translate result from a fan-out goroutine.
type translationResult struct {
	seq        int
	translated string
	skipped    bool // true if translation failed or phrase was invalid
}

// TranslateStream implements driven.Translator.
//
// Flow:
//  1. STT WebSocket produces transcripts on sttCh.
//  2. Each transcript is validated (language check) and assigned a sequence number.
//  3. A goroutine is launched per phrase to call GPT-4o-mini concurrently.
//  4. An ordered drainer collects results and emits them in sequence order,
//     then synthesizes TTS sequentially to preserve audio playback order.
func (p *PipelineTranslator) TranslateStream(
	ctx context.Context,
	audioIn <-chan []byte,
	sourceLang, targetLang string,
) (driven.TranslateResult, error) {
	transcriptCh := make(chan string, 4)
	audioCh := make(chan []byte, 8)

	sttCh, err := p.stt.Transcribe(ctx, audioIn, sourceLang)
	if err != nil {
		return driven.TranslateResult{}, err
	}

	resultCh := make(chan translationResult, 16)
	var translateWG sync.WaitGroup

	// Fan-out: one goroutine per accepted transcript, translate concurrently.
	go func() {
		seq := 0
		for {
			select {
			case <-ctx.Done():
				translateWG.Wait()
				close(resultCh)
				return
			case original, ok := <-sttCh:
				if !ok {
					translateWG.Wait()
					close(resultCh)
					return
				}
				if original == "" {
					continue
				}
				if !p.isExpectedLanguage(original, sourceLang) {
					slog.Warn("pipeline_lang_mismatch_dropped", "expected", sourceLang, "text", original)
					continue
				}
				mySeq := seq
				seq++
				translateWG.Add(1)
				go func(s int, text string) {
					defer translateWG.Done()
					translated, err := p.translate.Translate(ctx, text, sourceLang, targetLang)
					if err != nil {
						slog.Warn("pipeline_translate_error", "err", err, "source", text)
						resultCh <- translationResult{seq: s, skipped: true}
						return
					}
					slog.Info("pipeline_translated", "src", sourceLang, "dst", targetLang, "original", text, "translated", translated)
					resultCh <- translationResult{seq: s, translated: translated}
				}(mySeq, original)
			}
		}
	}()

	// Ordered drainer: buffer out-of-order results, emit in sequence.
	// TTS runs sequentially inside the drainer to keep audio playback ordered.
	go func() {
		defer close(transcriptCh)
		defer close(audioCh)

		pending := make(map[int]translationResult)
		nextExpected := 0

		for result := range resultCh {
			pending[result.seq] = result
			for {
				r, ok := pending[nextExpected]
				if !ok {
					break
				}
				delete(pending, nextExpected)
				nextExpected++

				if r.skipped {
					continue
				}

				select {
				case transcriptCh <- r.translated:
				default:
				}

				pcmCh, err := p.ttsClient.Synthesize(ctx, r.translated, targetLang)
				if err != nil {
					slog.Warn("pipeline_tts_error", "err", err, "text", r.translated)
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

// isExpectedLanguage returns false if the text is confidently detected as a language
// different from sourceLang. Short texts (<20 runes) are always accepted — insufficient
// signal for reliable detection. Inconclusive detections also pass through.
func (p *PipelineTranslator) isExpectedLanguage(text, sourceLang string) bool {
	if len([]rune(text)) < 20 {
		return true
	}
	detected, ok := p.langDetector.DetectLanguageOf(text)
	if !ok {
		return true
	}
	return strings.EqualFold(detected.IsoCode639_1().String(), sourceLang)
}

// Compile-time check.
var _ driven.Translator = (*PipelineTranslator)(nil)
