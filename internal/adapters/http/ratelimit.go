package httpserver

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// bucket tracks the request count within the current fixed window.
type bucket struct {
	count       int
	windowStart time.Time
}

// RateLimiter implements a fixed-window rate limiter keyed by client IP.
// When limit == 0 the limiter is disabled and every request is allowed.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	limit   int
	window  time.Duration
	nowFn   func() time.Time
}

// NewRateLimiter creates a RateLimiter using the real wall clock.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return NewRateLimiterWithClock(limit, window, time.Now)
}

// NewRateLimiterWithClock creates a RateLimiter with an injectable clock,
// allowing deterministic tests to control the notion of "now".
func NewRateLimiterWithClock(limit int, window time.Duration, nowFn func() time.Time) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*bucket),
		limit:   limit,
		window:  window,
		nowFn:   nowFn,
	}
}

// Allow checks whether the given IP is within its rate-limit quota.
// Returns (true, 0) when the request is permitted.
// Returns (false, retryAfterSec) when the limit is exceeded.
// When limit == 0 the limiter is disabled; every call returns (true, 0).
func (rl *RateLimiter) Allow(ip string) (bool, int) {
	if rl.limit == 0 {
		return true, 0
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.nowFn()

	b, ok := rl.buckets[ip]
	if !ok {
		rl.buckets[ip] = &bucket{count: 1, windowStart: now}
		return true, 0
	}

	// Reset window if the current window has expired.
	if now.Sub(b.windowStart) >= rl.window {
		b.count = 1
		b.windowStart = now
		return true, 0
	}

	if b.count < rl.limit {
		b.count++
		return true, 0
	}

	// Limit exceeded — compute seconds until the window resets.
	windowEnd := b.windowStart.Add(rl.window)
	remaining := windowEnd.Sub(now)
	retryAfter := int(remaining.Seconds())
	if retryAfter < 1 {
		retryAfter = 1
	}
	return false, retryAfter
}

// clientIP extracts the real client IP from a request.
// Checks X-Forwarded-For first, then X-Real-IP, then falls back to RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can be a comma-separated list; take the first value.
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Middleware returns an http.Handler that enforces the rate limit.
// Rejected requests receive HTTP 429 with a Retry-After header and a JSON body:
//
//	{"error":"rate-limited","retry_after_seconds":N}
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		allowed, retryAfter := rl.Allow(ip)
		if !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":               "rate-limited",
				"retry_after_seconds": retryAfter,
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Cleanup removes bucket entries whose window started more than 2*window ago.
// The now parameter is the reference time (injected for testability).
func (rl *RateLimiter) Cleanup(now time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := now.Add(-2 * rl.window)
	for ip, b := range rl.buckets {
		if b.windowStart.Before(cutoff) {
			delete(rl.buckets, ip)
		}
	}
}

// StartCleanup launches a background goroutine that calls Cleanup on every
// interval tick. The goroutine exits when ctx is cancelled.
func (rl *RateLimiter) StartCleanup(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				rl.Cleanup(t)
			}
		}
	}()
}

// StartCleanupNotify is like StartCleanup but closes the done channel when the
// goroutine exits. Used in tests to observe goroutine lifecycle.
func (rl *RateLimiter) StartCleanupNotify(ctx context.Context, interval time.Duration, done chan<- struct{}) {
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				rl.Cleanup(t)
			}
		}
	}()
}

// ---------------------------------------------------------------------------
// Test-support helpers (exported so the _test package can reach them)
// ---------------------------------------------------------------------------

// BucketCount returns the number of tracked IP entries.
// Exported for white-box testing of cleanup logic.
func (rl *RateLimiter) BucketCount() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return len(rl.buckets)
}

// SetBucketWindowStart directly sets the windowStart for an existing bucket.
// Exported for white-box testing of window-reset behaviour.
// If the IP has no bucket yet, one is created with count == limit.
func (rl *RateLimiter) SetBucketWindowStart(ip string, t time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.buckets[ip]
	if !ok {
		b = &bucket{count: rl.limit}
		rl.buckets[ip] = b
	}
	b.windowStart = t
}
