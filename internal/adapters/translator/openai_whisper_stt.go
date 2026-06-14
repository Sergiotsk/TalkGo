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
	// defaultSessionModel is used in the WebSocket URL — gpt-realtime-whisper goes here too.
	// The transcription session type is signaled via session.update, not the URL model.
	defaultSessionModel       = "gpt-realtime-whisper"
	defaultTranscriptionModel = "gpt-realtime-whisper"
	defaultWhisperBaseURL     = "wss://api.openai.com/v1/realtime/transcription"
)

// WhisperSTTConfig holds configuration for the Whisper STT adapter.
type WhisperSTTConfig struct {
	APIKey             string
	SessionModel       string // realtime session model for URL — default: gpt-realtime
	TranscriptionModel string // transcription model in session.update — default: gpt-realtime-whisper
	BaseURL            string // default: wss://api.openai.com/v1/realtime
}

func (c *WhisperSTTConfig) applyDefaults() {
	if c.SessionModel == "" {
		c.SessionModel = defaultSessionModel
	}
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
type sttMessage struct {
	Type    string          `json:"type"`
	Session json.RawMessage `json:"session,omitempty"`
	Audio   string          `json:"audio,omitempty"`
	Delta   string          `json:"delta,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
	// Transcript field for transcription.completed events.
	Transcript *sttTranscript `json:"transcript,omitempty"`
}

type sttTranscript struct {
	Text string `json:"text"`
}

// sttSessionPayload is the nested session config for gpt-realtime-whisper.
// Type is omitted — the transcription session type is inferred from the model in the URL.
type sttSessionPayload struct {
	Audio *sttAudio `json:"audio"`
}

type sttAudio struct {
	Input sttInput `json:"input"`
}

type sttInput struct {
	Format        sttFormat        `json:"format"`
	Transcription sttTranscription `json:"transcription"`
}

type sttFormat struct {
	Type string `json:"type"`
	Rate int    `json:"rate"`
}

type sttTranscription struct {
	Model    string `json:"model"`
	Language string `json:"language,omitempty"`
}

// Transcribe connects to gpt-realtime-whisper, streams audioIn, and returns
// a channel of final transcript strings. The channel is closed when audioIn
// is exhausted or ctx is cancelled.
func (w *WhisperSTT) Transcribe(ctx context.Context, audioIn <-chan []byte, lang string) (<-chan string, error) {
	url := fmt.Sprintf("%s?model=%s", w.cfg.BaseURL, w.cfg.SessionModel)

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+w.cfg.APIKey)

	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, url, headers)
	if err != nil {
		return nil, fmt.Errorf("whisper stt: dial: %w", err)
	}

	// Send session.update with transcription config.
	sessionPayload, err := json.Marshal(sttSessionPayload{
		Audio: &sttAudio{
			Input: sttInput{
				Format: sttFormat{
					Type: "audio/pcm",
					Rate: 24000,
				},
				Transcription: sttTranscription{
					Model:    w.cfg.TranscriptionModel,
					Language: lang,
				},
			},
		},
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("whisper stt: marshal session: %w", err)
	}

	update := sttMessage{
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
				// Partial transcript — useful for future streaming display.
				// Not forwarded for now; we use completed events for quality.

			case "conversation.item.input_audio_transcription.completed":
				if msg.Transcript != nil && msg.Transcript.Text != "" {
					slog.Info("whisper_transcript", "lang", lang, "text", msg.Transcript.Text)
					select {
					case transcriptCh <- msg.Transcript.Text:
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
