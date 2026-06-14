// Package translator implements translation adapters for the TalkGo pipeline.
package translator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

const (
	// defaultTranscriptionModel is the model used in session.update input_audio_transcription.
	defaultTranscriptionModel = "gpt-realtime-whisper"
	defaultWhisperBaseURL     = "wss://api.openai.com/v1/realtime"
)

// WhisperSTTConfig holds configuration for the Whisper STT adapter.
type WhisperSTTConfig struct {
	APIKey             string
	TranscriptionModel string // default: gpt-realtime-whisper
	BaseURL            string // default: wss://api.openai.com/v1/realtime
}

func (c *WhisperSTTConfig) applyDefaults() {
	if c.TranscriptionModel == "" {
		c.TranscriptionModel = defaultTranscriptionModel
	}
	if c.BaseURL == "" {
		c.BaseURL = defaultWhisperBaseURL
	}
}

// WhisperSTT streams audio to gpt-realtime-whisper and returns transcript strings.
type WhisperSTT struct {
	cfg WhisperSTTConfig
}

// NewWhisperSTT creates a WhisperSTT adapter ready to use.
func NewWhisperSTT(cfg WhisperSTTConfig) *WhisperSTT {
	cfg.applyDefaults()
	return &WhisperSTT{cfg: cfg}
}

// sttMessage is the shape of WebSocket messages for the transcription API.
// transcript is a plain string in conversation.item.input_audio_transcription.completed.
type sttMessage struct {
	Type       string          `json:"type"`
	Session    json.RawMessage `json:"session,omitempty"`
	Audio      string          `json:"audio,omitempty"`
	Delta      string          `json:"delta,omitempty"`
	Error      json.RawMessage `json:"error,omitempty"`
	Transcript string          `json:"transcript,omitempty"` // plain string, not nested object
}

// sttSessionPayload matches the realtime session.update format.
// Used with ?intent=transcription endpoint — same session schema as regular realtime.
type sttSessionPayload struct {
	Type                    string              `json:"type"` // "transcription"
	InputAudioFormat        string              `json:"input_audio_format"`
	InputAudioTranscription sttTranscriptionCfg `json:"input_audio_transcription"`
	TurnDetection           sttTurnDetection    `json:"turn_detection"`
}

type sttTranscriptionCfg struct {
	Model    string `json:"model"`
	Language string `json:"language,omitempty"`
}

type sttTurnDetection struct {
	Type string `json:"type"` // "server_vad"
}

// Transcribe connects to gpt-realtime-whisper, streams audioIn, and returns
// a channel of final transcript strings. The channel is closed when audioIn
// is exhausted or ctx is cancelled.
func (w *WhisperSTT) Transcribe(ctx context.Context, audioIn <-chan []byte, lang string) (<-chan string, error) {
	url := fmt.Sprintf("%s?intent=transcription", w.cfg.BaseURL)

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+w.cfg.APIKey)

	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, url, headers)
	if err != nil {
		return nil, fmt.Errorf("whisper stt: dial: %w", err)
	}

	// Send session.update with transcription config.
	// pcm16 = PCM 16-bit signed LE — matches what OpusCodec decodes to at 24kHz.
	sessionPayload, err := json.Marshal(sttSessionPayload{
		Type:             "transcription",
		InputAudioFormat: "pcm16",
		InputAudioTranscription: sttTranscriptionCfg{
			Model:    w.cfg.TranscriptionModel,
			Language: lang,
		},
		TurnDetection: sttTurnDetection{Type: "server_vad"},
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("whisper stt: marshal session: %w", err)
	}

	update := struct {
		Type    string          `json:"type"`
		Session json.RawMessage `json:"session"`
	}{
		Type:    "session.update",
		Session: json.RawMessage(sessionPayload),
	}
	if err := conn.WriteJSON(update); err != nil {
		conn.Close()
		return nil, fmt.Errorf("whisper stt: send session.update: %w", err)
	}

	transcriptCh := make(chan string, 8)
	label := fmt.Sprintf("stt:%s", lang)
	slog.Info("whisper_stt_connected", "lang", lang)

	var receiverDone sync.WaitGroup
	receiverDone.Add(1)

	// Sender: forward audio frames to the WebSocket.
	go func() {
		defer func() {
			_ = conn.WriteMessage(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			)
			receiverDone.Wait()
			conn.Close()
		}()

		sentFrames := 0
		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-audioIn:
				if !ok {
					return
				}
				sentFrames++
				if sentFrames == 1 || sentFrames%500 == 0 {
					slog.Info("whisper_audio_sending", "direction", label, "frame", sentFrames, "bytes", len(frame))
				}
				msg := sttMessage{
					Type:  "input_audio_buffer.append",
					Audio: base64.StdEncoding.EncodeToString(frame),
				}
				if err := conn.WriteJSON(msg); err != nil {
					return
				}
			}
		}
	}()

	// Receiver: collect transcription events.
	go func() {
		defer receiverDone.Done()
		defer close(transcriptCh)

		for {
			if ctx.Err() != nil {
				return
			}
			var msg sttMessage
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			switch msg.Type {
			case "conversation.item.input_audio_transcription.delta":
				// Partial transcript — not forwarded; completed events are used for quality.

			case "conversation.item.input_audio_transcription.completed":
				if msg.Transcript != "" {
					slog.Info("whisper_transcript", "lang", lang, "text", msg.Transcript)
					select {
					case transcriptCh <- msg.Transcript:
					case <-ctx.Done():
						return
					}
				}

			case "error":
				var apiErr apiError
				_ = json.Unmarshal(msg.Error, &apiErr)
				if apiErr.Code == "unknown_parameter" || apiErr.Code == "invalid_value" {
					slog.Warn("whisper_stt_warning", "detail", string(msg.Error))
					continue
				}
				slog.Error("whisper_stt_error", "detail", string(msg.Error))
				return

			default:
				switch msg.Type {
				case "session.created":
					slog.Info("whisper_stt_ready", "lang", lang)
				case "session.updated",
					"input_audio_buffer.speech_started",
					"input_audio_buffer.speech_stopped",
					"input_audio_buffer.committed",
					"conversation.item.created":
					// expected — ignore
				default:
					slog.Info("whisper_stt_event", "type", msg.Type)
				}
			}
		}
	}()

	return transcriptCh, nil
}
