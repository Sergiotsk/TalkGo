package httpserver_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	httpserver "github.com/Sergiotsk/TalkGo/internal/adapters/http"
)

// TASK-051: TestServer_RateLimit_Rooms_429
// Creates a server with a roomLimiter of limit=1 and verifies the second
// POST /rooms from the same IP returns 429 with a Retry-After header.
// Covers REQ-RATE-01, REQ-UX-04.
func TestServer_RateLimit_Rooms_429(t *testing.T) {
	roomLimiter := httpserver.NewRateLimiter(1, time.Hour)

	srv := httpserver.NewServer(
		httpserver.DefaultConfig(),
		&mockRoomManager{},
		nil,
		roomLimiter,
		nil, // wsLimiter
	)

	body := func() *bytes.Buffer {
		return bytes.NewBufferString(`{"source_lang":"es","target_lang":"en"}`)
	}

	// First request: should succeed (201).
	req1 := httptest.NewRequest(http.MethodPost, "/rooms", body())
	req1.Header.Set("Content-Type", "application/json")
	req1.RemoteAddr = "1.2.3.4:9000"
	w1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w1, req1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first request: got %d, want %d", w1.Code, http.StatusCreated)
	}

	// Second request from same IP: must be rate-limited (429).
	req2 := httptest.NewRequest(http.MethodPost, "/rooms", body())
	req2.Header.Set("Content-Type", "application/json")
	req2.RemoteAddr = "1.2.3.4:9001"
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("second request: got %d, want %d", w2.Code, http.StatusTooManyRequests)
	}
	if w2.Header().Get("Retry-After") == "" {
		t.Error("second request: missing Retry-After header")
	}
}

// TASK-052: TestServer_RateLimit_WS_429
// Creates a server with a wsLimiter of limit=1 and verifies the second
// GET /ws/{roomID} from the same IP returns 429 before any WebSocket upgrade.
// Covers REQ-RATE-02, REQ-UX-04.
func TestServer_RateLimit_WS_429(t *testing.T) {
	wsLimiter := httpserver.NewRateLimiter(1, time.Hour)

	srv := httpserver.NewServer(
		httpserver.DefaultConfig(),
		&mockRoomManager{},
		nil,
		nil, // roomLimiter
		wsLimiter,
	)

	// First request: room exists (default mock returns nil error from RoomExists),
	// but hub is nil → 500. The rate limiter has not rejected it yet.
	req1 := httptest.NewRequest(http.MethodGet, "/ws/testroom", http.NoBody)
	req1.RemoteAddr = "5.6.7.8:1000"
	w1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w1, req1)
	// We just care it was NOT 429.
	if w1.Code == http.StatusTooManyRequests {
		t.Fatalf("first request should not be rate-limited, got 429")
	}

	// Second request from same IP: must be rejected by the rate limiter before handler.
	req2 := httptest.NewRequest(http.MethodGet, "/ws/testroom", http.NoBody)
	req2.RemoteAddr = "5.6.7.8:1001"
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("second request: got %d, want %d", w2.Code, http.StatusTooManyRequests)
	}
	if w2.Header().Get("Retry-After") == "" {
		t.Error("second request: missing Retry-After header")
	}
}

// TASK-053: TestServer_RateLimit_IndependentEndpoints
// Exhausts the rooms limiter and verifies the WS endpoint is still accessible
// (separate limiter bucket / separate limiter instance).
// Covers REQ-RATE-01, REQ-RATE-02.
func TestServer_RateLimit_IndependentEndpoints(t *testing.T) {
	roomLimiter := httpserver.NewRateLimiter(1, time.Hour)
	wsLimiter := httpserver.NewRateLimiter(5, time.Hour) // generous limit

	srv := httpserver.NewServer(
		httpserver.DefaultConfig(),
		&mockRoomManager{},
		nil,
		roomLimiter,
		wsLimiter,
	)

	ip := "9.9.9.9"

	// Exhaust rooms limiter: 1 allowed + 1 rejected.
	for i := 0; i < 2; i++ {
		body := bytes.NewBufferString(`{"source_lang":"es","target_lang":"en"}`)
		req := httptest.NewRequest(http.MethodPost, "/rooms", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":80"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
	}

	// Verify rooms is now blocked.
	body := bytes.NewBufferString(`{"source_lang":"es","target_lang":"en"}`)
	reqRooms := httptest.NewRequest(http.MethodPost, "/rooms", body)
	reqRooms.Header.Set("Content-Type", "application/json")
	reqRooms.RemoteAddr = ip + ":80"
	wRooms := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wRooms, reqRooms)
	if wRooms.Code != http.StatusTooManyRequests {
		t.Errorf("rooms limiter: got %d, want %d", wRooms.Code, http.StatusTooManyRequests)
	}

	// WS endpoint should still be accessible (not 429).
	reqWS := httptest.NewRequest(http.MethodGet, "/ws/testroom", http.NoBody)
	reqWS.RemoteAddr = ip + ":80"
	wWS := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wWS, reqWS)
	if wWS.Code == http.StatusTooManyRequests {
		t.Error("ws endpoint should NOT be rate-limited when only roomLimiter is exhausted")
	}
}
