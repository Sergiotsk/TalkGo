// Package tts implements the TextToSpeech driven port using the OpenAI TTS API.
package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
)

const (
	defaultTTSModel   = "gpt-4o-mini-tts-2025-12-15"
	defaultTTSBaseURL = "https://api.openai.com/v1/audio/speech"
	// defaultVoice is the OpenAI TTS voice used for all output.
	// "alloy" is neutral and works well for translated speech.
	defaultVoice = "alloy"
)

// Config holds configuration for the OpenAI TTS adapter.
type Config struct {
	APIKey  string
	Model   string // default: gpt-4o-mini-tts-2025-12-15
	Voice   string // default: alloy
	BaseURL string // default: https://api.openai.com/v1/audio/speech
}

func (c *Config) applyDefaults() {
	if c.Model == "" {
		c.Model = defaultTTSModel
	}
	if c.Voice == "" {
		c.Voice = defaultVoice
	}
	if c.BaseURL == "" {
		c.BaseURL = defaultTTSBaseURL
	}
}

// OpenAITTS implements driven.TextToSpeech using the OpenAI TTS REST API.
// It requests PCM16 output at 24 kHz mono to match the codec pipeline.
type OpenAITTS struct {
	cfg    Config
	client *http.Client
}

// NewOpenAITTS creates an OpenAITTS adapter ready to use.
func NewOpenAITTS(cfg Config) *OpenAITTS {
	cfg.applyDefaults()
	return &OpenAITTS{cfg: cfg, client: &http.Client{}}
}

type ttsRequest struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format"`
}

// Synthesize calls the OpenAI TTS API and streams the PCM16 response on the
// returned channel. The channel carries a single chunk with all PCM bytes and
// is then closed. Returns an error if the HTTP request fails.
func (t *OpenAITTS) Synthesize(ctx context.Context, text, lang string) (<-chan []byte, error) {
	body, err := json.Marshal(ttsRequest{
		Model:          t.cfg.Model,
		Input:          text,
		Voice:          t.cfg.Voice,
		ResponseFormat: "pcm", // PCM16 LE 24kHz mono — matches codec pipeline
	})
	if err != nil {
		return nil, fmt.Errorf("tts: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tts: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tts: http request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("tts: api error %d: %s", resp.StatusCode, string(body))
	}

	out := make(chan []byte, 1)
	go func() {
		defer close(out)
		defer resp.Body.Close()

		pcm, err := io.ReadAll(resp.Body)
		if err != nil {
			slog.Warn("tts: read response body", "err", err)
			return
		}
		if len(pcm) == 0 {
			slog.Warn("tts: empty response body")
			return
		}
		slog.Info("tts_synthesized", "lang", lang, "text_len", len(text), "pcm_bytes", len(pcm))
		select {
		case out <- pcm:
		case <-ctx.Done():
		}
	}()

	return out, nil
}

// Compile-time check.
var _ driven.TextToSpeech = (*OpenAITTS)(nil)
