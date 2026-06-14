package translator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
)

const (
	defaultModel   = "gpt-4o-realtime-preview"
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
}

// sessionUpdate is the payload for session.update messages.
type sessionUpdate struct {
	Instructions      string `json:"instructions"`
	InputAudioFormat  string `json:"input_audio_format"`
	OutputAudioFormat string `json:"output_audio_format"`
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
	headers.Set("OpenAI-Beta", "realtime=v1")

	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, url, headers)
	if err != nil {
		return driven.TranslateResult{}, fmt.Errorf("openai realtime: dial %s: %w", url, err)
	}

	// Send session.update to configure translation behaviour.
	sessionPayload, err := json.Marshal(sessionUpdate{
		Instructions:      fmt.Sprintf("Translate from %s to %s. Output only the translation.", sourceLang, targetLang),
		InputAudioFormat:  "pcm16",
		OutputAudioFormat: "pcm16",
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

		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-audioIn:
				if !ok {
					return
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
			case "response.audio.delta":
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
			case "response.audio_transcript.delta":
				transcriptBuf += msg.Delta
			case "response.audio_transcript.done":
				if transcriptBuf != "" {
					select {
					case transcriptCh <- transcriptBuf:
					default:
					}
					transcriptBuf = ""
				}
			default:
				// Log unexpected event types to diagnose missing transcripts.
				if msg.Type != "session.created" && msg.Type != "session.updated" &&
					msg.Type != "response.created" && msg.Type != "response.done" &&
					msg.Type != "response.output_item.added" && msg.Type != "response.output_item.done" &&
					msg.Type != "response.content_part.added" && msg.Type != "response.content_part.done" &&
					msg.Type != "input_audio_buffer.speech_started" && msg.Type != "input_audio_buffer.speech_stopped" &&
					msg.Type != "input_audio_buffer.committed" && msg.Type != "rate_limits.updated" &&
					msg.Type != "conversation.item.created" {
					slog.Info("openai_realtime_event", "type", msg.Type)
				}
			}
		}
	}()

	return driven.TranslateResult{Audio: audioCh, Transcript: transcriptCh}, nil
}
