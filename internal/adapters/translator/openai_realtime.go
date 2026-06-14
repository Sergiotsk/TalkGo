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

	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
)

const (
	defaultModel   = "gpt-realtime"
	defaultBaseURL = "wss://api.openai.com/v1/realtime"
)

// OpenAIRealtimeConfig holds configuration for the OpenAI Realtime Translate API.
type OpenAIRealtimeConfig struct {
	APIKey  string
	Model   string // default: "gpt-4o-realtime-preview"
	BaseURL string // default: "wss://api.openai.com/v1/realtime"
}

func (c *OpenAIRealtimeConfig) applyDefaults() {
	if c.Model == "" {
		c.Model = defaultModel
	}
	if c.BaseURL == "" {
		c.BaseURL = defaultBaseURL
	}
}

// OpenAIRealtimeTranslator implements driven.Translator using the OpenAI Realtime API.
type OpenAIRealtimeTranslator struct {
	cfg OpenAIRealtimeConfig
}

// NewOpenAIRealtimeTranslator creates a Translator backed by the OpenAI Realtime API.
func NewOpenAIRealtimeTranslator(cfg OpenAIRealtimeConfig) *OpenAIRealtimeTranslator {
	cfg.applyDefaults()
	return &OpenAIRealtimeTranslator{cfg: cfg}
}

// wsMessage represents a JSON message sent to or received from the OpenAI Realtime API.
type wsMessage struct {
	Type    string          `json:"type"`
	Session json.RawMessage `json:"session,omitempty"`
	Audio   string          `json:"audio,omitempty"`
	Delta   string          `json:"delta,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
	Part    json.RawMessage `json:"part,omitempty"`
}

// contentPart is the shape of the "part" field in response.content_part.done.
type contentPart struct {
	Type       string `json:"type"`
	Audio      string `json:"audio"`
	Transcript string `json:"transcript"`
	Text       string `json:"text"`
}

// apiError is the shape of the "error" payload from OpenAI.
type apiError struct {
	Code string `json:"code"`
}

// sessionUpdate is the payload for session.update messages.
type sessionUpdate struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
}

// TranslateStream connects to the OpenAI Realtime API and streams translated audio and transcripts.
// It returns an error immediately if the initial WebSocket connection fails.
// Both channels are closed when audioIn is exhausted or ctx is cancelled.
func (t *OpenAIRealtimeTranslator) TranslateStream(
	ctx context.Context,
	audioIn <-chan []byte,
	sourceLang, targetLang string,
) (driven.TranslateResult, error) {
	url := fmt.Sprintf("%s?model=%s", t.cfg.BaseURL, t.cfg.Model)

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+t.cfg.APIKey)

	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, url, headers)
	if err != nil {
		return driven.TranslateResult{}, fmt.Errorf("openai realtime: dial %s: %w", url, err)
	}

	// Send session.update to configure translation behaviour.
	sessionPayload, err := json.Marshal(sessionUpdate{
		Type: "realtime",
		Instructions: fmt.Sprintf(
			"You are a real-time speech translator. Your ONLY task is to listen to speech in %s and immediately speak the translation in %s. "+
				"NEVER respond as an AI assistant. NEVER greet, acknowledge, or chat. NEVER speak in %s. "+
				"ONLY output the spoken translation in %s. If you hear noise or silence, stay silent.",
			sourceLang, targetLang, sourceLang, targetLang,
		),
	})
	if err != nil {
		conn.Close()
		return driven.TranslateResult{}, fmt.Errorf("openai realtime: marshal session.update: %w", err)
	}

	update := wsMessage{
		Type:    "session.update",
		Session: json.RawMessage(sessionPayload),
	}
	if err := conn.WriteJSON(update); err != nil {
		conn.Close()
		return driven.TranslateResult{}, fmt.Errorf("openai realtime: send session.update: %w", err)
	}

	audioCh := make(chan []byte, 8)
	transcriptCh := make(chan string, 4)

	label := fmt.Sprintf("%s→%s", sourceLang, targetLang)
	slog.Info("openai_realtime_connected", "direction", label)

	// senderDone signals the receiver that no more audio will be written.
	var receiverDone sync.WaitGroup
	receiverDone.Add(1)

	// Sender goroutine: drain audioIn and forward frames to the WebSocket.
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
					slog.Info("openai_audio_sending", "direction", label, "frame", sentFrames, "bytes", len(frame))
				}
				msg := wsMessage{
					Type:  "input_audio_buffer.append",
					Audio: base64.StdEncoding.EncodeToString(frame),
				}
				if err := conn.WriteJSON(msg); err != nil {
					return
				}
			}
		}
	}()

	// Receiver goroutine: read translated audio deltas and transcripts from the WebSocket.
	go func() {
		defer receiverDone.Done()
		defer close(audioCh)
		defer close(transcriptCh)
		var transcriptBuf string
		for {
			if ctx.Err() != nil {
				return
			}
			var msg wsMessage
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			switch msg.Type {
			// Streaming audio path (gpt-4o-realtime and similar).
			case "response.output_audio.delta":
				if msg.Delta == "" {
					continue
				}
				decoded, err := base64.StdEncoding.DecodeString(msg.Delta)
				if err != nil {
					continue
				}
				select {
				case audioCh <- decoded:
				case <-ctx.Done():
					return
				}
			case "response.output_audio_transcript.delta":
				transcriptBuf += msg.Delta
			case "response.output_audio_transcript.done":
				if transcriptBuf != "" {
					select {
					case transcriptCh <- transcriptBuf:
					default:
					}
					transcriptBuf = ""
				}
			// Batch audio path (gpt-realtime): full audio packed in content_part.done.
			case "response.content_part.done":
				if len(msg.Part) == 0 {
					continue
				}
				var part contentPart
				if err := json.Unmarshal(msg.Part, &part); err != nil {
					slog.Warn("openai_realtime: failed to parse content_part", "err", err)
					continue
				}
				slog.Info("openai_realtime_content_part", "type", part.Type, "audio_len", len(part.Audio), "transcript", part.Transcript)
				if part.Audio != "" {
					decoded, err := base64.StdEncoding.DecodeString(part.Audio)
					if err == nil && len(decoded) > 0 {
						select {
						case audioCh <- decoded:
						case <-ctx.Done():
							return
						}
					}
				}
				text := part.Transcript
				if text == "" {
					text = part.Text
				}
				if text != "" {
					select {
					case transcriptCh <- text:
					default:
					}
				}
			case "error":
				var apiErr apiError
				_ = json.Unmarshal(msg.Error, &apiErr)
				// Non-fatal: bad session param — session stays alive, log and continue.
				if apiErr.Code == "unknown_parameter" || apiErr.Code == "invalid_value" {
					slog.Warn("openai_realtime_session_warning", "detail", string(msg.Error))
					continue
				}
				// Fatal: auth failure, quota exceeded, etc.
				slog.Error("openai_realtime_error", "detail", string(msg.Error))
				return
			default:
				// Silence known no-op events; log anything unexpected.
				switch msg.Type {
				case "session.created":
					slog.Info("openai_session_ready", "direction", label)
				case "session.updated",
					"response.created", "response.done",
					"response.output_audio.done",
					"conversation.item.added", "conversation.item.done",
					"input_audio_buffer.speech_started", "input_audio_buffer.speech_stopped",
					"input_audio_buffer.committed", "rate_limits.updated":
					// expected — ignore
				default:
					slog.Info("openai_realtime_event", "type", msg.Type)
				}
			}
		}
	}()

	return driven.TranslateResult{Audio: audioCh, Transcript: transcriptCh}, nil
}
