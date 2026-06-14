package translator_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Sergiotsk/TalkGo/internal/adapters/translator"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// wsMessage mirrors the adapter's internal message shape for test decoding.
type wsMessage struct {
	Type    string          `json:"type"`
	Session json.RawMessage `json:"session,omitempty"`
	Audio   string          `json:"audio,omitempty"`
	Delta   string          `json:"delta,omitempty"`
}

// newEchoServer returns an httptest.Server that:
//  1. Upgrades the connection to WebSocket.
//  2. Reads session.update (ignored — just consumed).
//  3. For each input_audio_buffer.append it receives, sends back a
//     response.output_audio.delta with the same base64-encoded payload (echo).
func newEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("mock server: upgrade error: %v", err)
			return
		}
		defer conn.Close()

		for {
			var msg wsMessage
			if err := conn.ReadJSON(&msg); err != nil {
				// Client closed — normal shutdown.
				return
			}

			switch msg.Type {
			case "session.update":
				// Nothing to echo — just consume.
			case "input_audio_buffer.append":
				reply := wsMessage{
					Type:  "response.output_audio.delta",
					Delta: msg.Audio, // echo the same base64 payload
				}
				if err := conn.WriteJSON(reply); err != nil {
					return
				}
			}
		}
	}))
}

func TestOpenAIRealtimeTranslator_TranslateStream(t *testing.T) {
	srv := newEchoServer(t)
	defer srv.Close()

	// httptest uses "http://", we need "ws://".
	wsURL := "ws://" + strings.TrimPrefix(srv.URL, "http://")

	cfg := translator.OpenAIRealtimeConfig{
		APIKey:  "test-key",
		Model:   "test-model",
		BaseURL: wsURL,
	}
	tr := translator.NewOpenAIRealtimeTranslator(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	audioFrame := []byte("test-audio-frame")
	audioIn := make(chan []byte, 1)
	audioIn <- audioFrame
	close(audioIn)

	result, err := tr.TranslateStream(ctx, audioIn, "es", "en")
	require.NoError(t, err)

	select {
	case frame, ok := <-result.Audio:
		require.True(t, ok, "output channel should deliver a frame before closing")
		// The echo server returns the same base64 audio, so decoding gives us back the original bytes.
		decoded, decErr := base64.StdEncoding.DecodeString(base64.StdEncoding.EncodeToString(audioFrame))
		require.NoError(t, decErr)
		assert.Equal(t, decoded, frame, "received frame should match the sent audio")
	case <-ctx.Done():
		t.Fatal("timed out waiting for translated frame")
	}
}

func TestOpenAIRealtimeTranslator_ConnectionFailure(t *testing.T) {
	cfg := translator.OpenAIRealtimeConfig{
		BaseURL: "ws://localhost:1", // nothing listening
		APIKey:  "key",
	}
	tr := translator.NewOpenAIRealtimeTranslator(cfg)

	ctx := context.Background()
	audioIn := make(chan []byte)
	close(audioIn)

	_, err := tr.TranslateStream(ctx, audioIn, "es", "en")
	assert.Error(t, err, "expected connection error, got nil")
}

func TestOpenAIRealtimeTranslator_ContextCancellation(t *testing.T) {
	srv := newEchoServer(t)
	defer srv.Close()

	wsURL := "ws://" + strings.TrimPrefix(srv.URL, "http://")

	cfg := translator.OpenAIRealtimeConfig{
		APIKey:  "test-key",
		Model:   "test-model",
		BaseURL: wsURL,
	}
	tr := translator.NewOpenAIRealtimeTranslator(cfg)

	ctx, cancel := context.WithCancel(context.Background())

	// audioIn never closes — the context cancellation is the only exit signal.
	audioIn := make(chan []byte)

	result, err := translator.NewOpenAIRealtimeTranslator(cfg).TranslateStream(ctx, audioIn, "en", "es")
	_ = tr // silence unused warning
	require.NoError(t, err)

	// Cancel immediately.
	cancel()

	// The output channel must be closed eventually (goroutines cleaned up).
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range result.Audio {
		}
	}()

	select {
	case <-done:
		// All good — output channel was closed.
	case <-time.After(3 * time.Second):
		t.Fatal("output channel not closed after context cancellation")
	}
}
