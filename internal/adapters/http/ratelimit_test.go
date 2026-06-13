package httpserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	httpserver "github.com/Sergiotsk/TalkGo/internal/adapters/http"
)

// ---------------------------------------------------------------------------
// TASK-021: Allow — under limit, all calls return (true, 0)
// ---------------------------------------------------------------------------

func TestRateLimiter_Allow_UnderLimit(t *testing.T) {
	const limit = 5
	rl := httpserver.NewRateLimiter(limit, time.Minute)

	for i := 0; i < limit; i++ {
		allowed, retry := rl.Allow("1.2.3.4")
		if !allowed {
			t.Fatalf("call %d: expected allowed=true, got false", i+1)
		}
		if retry != 0 {
			t.Fatalf("call %d: expected retryAfter=0, got %d", i+1, retry)
		}
	}
}

// ---------------------------------------------------------------------------
// TASK-022: Allow — at limit, limit+1 call returns (false, retryAfter > 0)
// ---------------------------------------------------------------------------

func TestRateLimiter_Allow_AtLimit(t *testing.T) {
	const limit = 3
	rl := httpserver.NewRateLimiter(limit, time.Minute)

	for i := 0; i < limit; i++ {
		allowed, _ := rl.Allow("1.2.3.4")
		if !allowed {
			t.Fatalf("call %d should be allowed", i+1)
		}
	}

	allowed, retry := rl.Allow("1.2.3.4")
	if allowed {
		t.Fatal("expected allowed=false after exceeding limit")
	}
	if retry <= 0 {
		t.Fatalf("expected retryAfter > 0, got %d", retry)
	}
}

// ---------------------------------------------------------------------------
// TASK-023: Allow — independent IPs have separate buckets
// ---------------------------------------------------------------------------

func TestRateLimiter_Allow_IndependentIPs(t *testing.T) {
	const limit = 2
	rl := httpserver.NewRateLimiter(limit, time.Minute)

	// Fill bucket for IP-A
	for i := 0; i < limit+1; i++ {
		rl.Allow("1.1.1.1") //nolint:errcheck
	}

	// IP-B must still be allowed
	allowed, retry := rl.Allow("2.2.2.2")
	if !allowed {
		t.Fatal("IP-B should be allowed when only IP-A bucket is full")
	}
	if retry != 0 {
		t.Fatalf("expected retryAfter=0 for IP-B, got %d", retry)
	}
}

// ---------------------------------------------------------------------------
// TASK-024: Allow — disabled limiter (limit=0) always allows
// ---------------------------------------------------------------------------

func TestRateLimiter_Allow_Disabled(t *testing.T) {
	rl := httpserver.NewRateLimiter(0, time.Minute)

	for i := 0; i < 100; i++ {
		allowed, retry := rl.Allow("1.2.3.4")
		if !allowed {
			t.Fatalf("call %d: disabled limiter should always allow", i+1)
		}
		if retry != 0 {
			t.Fatalf("call %d: disabled limiter should return retryAfter=0, got %d", i+1, retry)
		}
	}
}

// ---------------------------------------------------------------------------
// TASK-025: RetryAfter — value reflects remaining window time correctly
// ---------------------------------------------------------------------------

func TestRateLimiter_RetryAfter_Correct(t *testing.T) {
	const limit = 2
	window := 60 * time.Second

	// t=0: create limiter with a fixed now
	fakeNow := time.Now()
	rl := httpserver.NewRateLimiterWithClock(limit, window, func() time.Time { return fakeNow })

	// Fill bucket at t=0
	for i := 0; i < limit; i++ {
		rl.Allow("1.2.3.4") //nolint:errcheck
	}

	// Advance clock 10s into the 60s window — 50s remain
	fakeNow = fakeNow.Add(10 * time.Second)

	allowed, retry := rl.Allow("1.2.3.4")
	if allowed {
		t.Fatal("expected denied after filling bucket")
	}

	// retryAfter should be ≈50s (within 2s tolerance)
	const wantApprox = 50
	const tolerance = 2
	if retry < wantApprox-tolerance || retry > wantApprox+tolerance {
		t.Fatalf("expected retryAfter ≈ %ds (±%ds), got %ds", wantApprox, tolerance, retry)
	}
}

// ---------------------------------------------------------------------------
// TASK-026: Cleanup — removes stale entries (windowStart > 2*window ago)
// ---------------------------------------------------------------------------

func TestRateLimiter_Cleanup_RemovesStale(t *testing.T) {
	window := 10 * time.Second
	rl := httpserver.NewRateLimiter(5, window)

	// Seed some entries
	rl.Allow("10.0.0.1") //nolint:errcheck
	rl.Allow("10.0.0.2") //nolint:errcheck
	rl.Allow("10.0.0.3") //nolint:errcheck

	if rl.BucketCount() == 0 {
		t.Fatal("expected buckets to be populated before cleanup")
	}

	// Cleanup with now = 3*window in the future (stale)
	future := time.Now().Add(3 * window)
	rl.Cleanup(future)

	if n := rl.BucketCount(); n != 0 {
		t.Fatalf("expected 0 buckets after cleanup, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// TASK-027: Allow — window reset when windowStart is in the past
// ---------------------------------------------------------------------------

func TestRateLimiter_Allow_WindowReset(t *testing.T) {
	const limit = 2
	window := 60 * time.Second
	rl := httpserver.NewRateLimiter(limit, window)

	// Fill bucket at current time
	for i := 0; i < limit; i++ {
		rl.Allow("5.5.5.5") //nolint:errcheck
	}

	// Verify bucket is exhausted
	allowed, _ := rl.Allow("5.5.5.5")
	if allowed {
		t.Fatal("bucket should be full before window reset injection")
	}

	// Inject a past windowStart directly to simulate window expiry
	past := time.Now().Add(-(window + time.Second))
	rl.SetBucketWindowStart("5.5.5.5", past)

	// Now Allow should reset the window and permit the request
	allowed, retry := rl.Allow("5.5.5.5")
	if !allowed {
		t.Fatal("expected allowed=true after window reset")
	}
	if retry != 0 {
		t.Fatalf("expected retryAfter=0 after window reset, got %d", retry)
	}
}

// ---------------------------------------------------------------------------
// TASK-028: Middleware — 429 response with Retry-After header and JSON body
// ---------------------------------------------------------------------------

func TestRateLimiter_Middleware_429Response(t *testing.T) {
	// limit=1 so the second request hits 429
	rl := httpserver.NewRateLimiter(1, time.Minute)

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := rl.Middleware(next)

	newReq := func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.RemoteAddr = "192.168.1.1:1234"
		return r
	}

	// First request — must pass through
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, newReq())
	if w1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", w1.Code)
	}

	// Second request — must be rejected
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, newReq())

	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w2.Code)
	}

	retryAfter := w2.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("expected Retry-After header to be set")
	}

	var body struct {
		Error            string `json:"error"`
		RetryAfterSeconds int   `json:"retry_after_seconds"`
	}
	if err := json.NewDecoder(w2.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Error != "rate-limited" {
		t.Errorf("expected error=rate-limited, got %q", body.Error)
	}
	if body.RetryAfterSeconds <= 0 {
		t.Errorf("expected retry_after_seconds > 0, got %d", body.RetryAfterSeconds)
	}
}

// ---------------------------------------------------------------------------
// TASK-030: StartCleanup — goroutine exits on context cancel
// ---------------------------------------------------------------------------

func TestRateLimiter_StartCleanup_ExitsOnCancel(t *testing.T) {
	rl := httpserver.NewRateLimiter(10, time.Minute)

	before := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	rl.StartCleanupNotify(ctx, 50*time.Millisecond, done)

	// Goroutine count should increase by at least 1
	// Give it a moment to start
	time.Sleep(20 * time.Millisecond)

	cancel()

	select {
	case <-done:
		// goroutine exited cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup goroutine did not exit after context cancel")
	}

	// Allow scheduler to reclaim the goroutine
	time.Sleep(20 * time.Millisecond)
	after := runtime.NumGoroutine()

	// Goroutine count must not have grown (allow ±2 for test scheduler noise)
	if after > before+2 {
		t.Errorf("possible goroutine leak: before=%d, after=%d", before, after)
	}
}
