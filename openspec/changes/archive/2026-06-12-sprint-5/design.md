# Sprint 5 Design — Alpha con Usuarios Finales

**Change**: sprint-5
**Status**: design
**Date**: 2026-06-12

---

## Architecture Overview

Sprint 5 replaces development stubs with production components and adds operational guardrails. The hexagonal architecture stays intact — every change is either a new adapter, a modification to an existing adapter, or a new middleware. No domain or port interfaces change.

**What changes:**
- `internal/adapters/codec/` — New `OpusCodec` adapter alongside existing `PassthroughCodec`
- `internal/adapters/webrtc/` — `Config` struct gains TURN support via a builder function
- `internal/adapters/http/` — Rate limiter middleware, feedback handler, extended health check
- `cmd/server/main.go` — Centralized `loadConfig()`, codec selection, TURN wiring
- Root — `Dockerfile`, `docker-compose.yml`, `Caddyfile`, `deploy/coturn/turnserver.conf`

**What stays the same:**
- All port interfaces (`driven.AudioCodec`, `driven.WebRTCPeer`, `driven.Translator`, `driven.EventNotifier`, `driven.RoomRepository`, `driving.RoomManager`, `driving.SignalingHandler`)
- Domain entities (`room.Room`, `session.Session`)
- Service layer (`roomsvc.Service`, `roomsvc.pipeline`)
- Signaling hub (`signaling.Hub`, `signaling.Client`)
- Translation adapter (`translator.OpenAIRealtimeTranslator`)

---

## Design Decisions

### D-01: OpusCodec uses hraban/opus (CGO, libopus)

**Decision**: Implement `OpusCodec` using `gopkg.in/hraban/opus.v2` for both encoding and decoding at 24kHz mono.

**Rationale**: The current `PassthroughCodec` (`internal/adapters/codec/opus_codec.go:28`) forwards frames unchanged. The `driven.AudioCodec` interface already defines the correct channel-based signatures (`Decode(ctx, <-chan []byte) (<-chan []byte, error)` and `Encode(ctx, <-chan []byte) (<-chan []byte, error)`) at `internal/ports/driven/audio_codec.go:7-15`. `pion/opus v0.1.0` was investigated first but rejected because it has no public `Encoder` type — only a decoder is exposed. `hraban/opus` provides a complete, production-tested encoder+decoder pair backed by `libopus`.

**Alternatives rejected**:
- `github.com/pion/opus` — The v0.1.0 release exposes no public `Encoder`. Only decoding is possible via the public API; encoding requires internal types not part of the public contract. Cannot be used.
- Build tags (`//go:build opus`) to conditionally compile — Overengineered for 2 codec implementations. A runtime env var (`CODEC_MODE`) is simpler and doesn't require separate build artifacts.

**Impact**: New file `internal/adapters/codec/opus.go`, modified `cmd/server/main.go`.

---

### D-02: CODEC_MODE env var selects codec at runtime (not build tags)

**Decision**: `main.go` reads `CODEC_MODE` from `os.Getenv`. Default: `"opus"`. Valid values: `"opus"`, `"passthrough"`. Unknown values cause `os.Exit(1)` with a clear error log.

**Rationale**: The `PassthroughCodec` must remain available for local development on Windows where `pion/opus` may have edge cases, and for fast integration tests that don't need real codec processing. Build tags would require maintaining two build configurations. An env var switch is one `switch` statement in `main.go`.

**Alternatives rejected**:
- Build tags — Require separate `go build` invocations and make CI more complex.
- Constructor flag on a single codec struct — Mixes passthrough behavior into production codec code.

**Impact**: `cmd/server/main.go` (line ~68-75 area, codec instantiation).

---

### D-03: PassthroughCodec renamed to its own file

**Decision**: Rename `internal/adapters/codec/opus_codec.go` to `internal/adapters/codec/passthrough.go` and rename `internal/adapters/codec/opus_codec_test.go` to `internal/adapters/codec/passthrough_test.go`. The new real `OpusCodec` lives in `internal/adapters/codec/opus.go` with tests in `internal/adapters/codec/opus_test.go`.

**Rationale**: The current file naming is misleading — `opus_codec.go` contains `PassthroughCodec`. The spec (REQ-COD-04) explicitly states `PassthroughCodec` should live in `passthrough.go`. Separating the files makes the codebase self-documenting.

**Alternatives rejected**:
- Keep both in the same file — Confusing; a file called `opus_codec.go` should contain the real Opus implementation.

**Impact**: File rename (no logic change) + new `opus.go` file.

---

### D-04: OpusCodec is synchronous per-frame, channels handled externally

**Decision**: `OpusCodec` wraps `pion/opus.Decoder` and `pion/opus.Encoder` instances. Each `Decode`/`Encode` method spawns a goroutine that reads from the input channel, processes frames synchronously, and writes to the output channel. The goroutine exits when the input channel closes or the context is cancelled.

**Rationale**: This matches the existing `PassthroughCodec` pattern (`internal/adapters/codec/opus_codec.go:47-68` — the `passthrough` helper). The pipeline in `runHalf` (`internal/app/roomsvc/pipeline.go:137-237`) already consumes channels, so the codec must produce channels.

**Key parameters**:
- Sample rate: 24,000 Hz (matches OpenAI Realtime API's `pcm16` format)
- Channels: 1 (mono)
- Frame size: 480 samples (20ms at 24kHz) — standard Opus frame size
- Application: `opus.AppVoIP` (optimized for speech)

**Impact**: New `internal/adapters/codec/opus.go`.

---

### D-05: TURN configuration is additive — empty env vars preserve current behavior

**Decision**: Add a `BuildICEConfig(turnURLs, turnUser, turnPass string) Config` function in `internal/adapters/webrtc/`. When `turnURLs` is empty, it returns the current STUN-only config (identical to `DefaultConfig()` at `internal/adapters/webrtc/pion_peer.go:23-28`). When non-empty, it appends TURN servers to the existing STUN server.

**Rationale**: Backward compatibility is critical. Every existing test and every developer running locally must see zero behavior change when TURN env vars are absent. The current `DefaultConfig()` works; we don't modify it, we add a higher-level builder.

**Alternatives rejected**:
- Modify `DefaultConfig()` to read env vars directly — Violates separation of concerns (adapter shouldn't read env vars; that's main.go's job).
- Make TURN mandatory — Breaks local development.

**Impact**: New function in `internal/adapters/webrtc/pion_peer.go` (or new file `config.go`), modified `cmd/server/main.go`.

---

### D-06: TURN_URLS supports comma-separated multiple URLs

**Decision**: `TURN_URLS` is parsed as a comma-separated list (e.g., `turn:srv1:3478,turn:srv2:3478`). All URLs share the same `TURN_USERNAME` and `TURN_PASSWORD` and are added as a single `ICEServer` entry (Pion supports multiple URLs per `ICEServer`).

**Rationale**: Coturn can listen on multiple ports/protocols (UDP, TCP, TLS). A single env var with comma separation is simpler than multiple indexed vars (`TURN_URL_1`, `TURN_URL_2`).

**Alternatives rejected**:
- JSON array — Harder to set in Docker environment variables.
- Multiple env vars — Verbose, no clear terminator.

**Impact**: Parsing logic in `cmd/server/main.go` (`loadConfig`).

---

### D-07: Rate limiter uses fixed-window token bucket with sync.Mutex

**Decision**: Implement a `RateLimiter` struct in `internal/adapters/http/ratelimit.go` using a fixed-window counter per IP. Each IP gets a `bucket` struct with `count int`, `windowStart time.Time`. When a request arrives: if `now - windowStart > window`, reset counter to 0 and update `windowStart`. If `count >= limit`, reject with 429. The map is `map[string]*bucket` protected by `sync.Mutex`.

**Rationale**: At 5 users, a simple fixed-window counter is sufficient. Token bucket with smooth refill is more complex and unnecessary. The stdlib approach (no `golang.org/x/time/rate`) keeps the zero-dependency constraint. The existing codebase uses `sync.Mutex` extensively (`roomsvc.Service` at `service.go:56`, `LatencyTracker` at `latency.go:48`) — the pattern is familiar.

**Key design**:
```go
type bucket struct {
    count       int
    windowStart time.Time
}

type RateLimiter struct {
    mu      sync.Mutex
    buckets map[string]*bucket
    limit   int
    window  time.Duration
}
```

**Alternatives rejected**:
- `golang.org/x/time/rate` — External dependency. The spec (REQ-RATE-04) explicitly forbids new deps.
- Sliding window — More complex, marginal benefit at 5 users.
- Middleware at Caddy level — Less granular, no per-endpoint control, harder to test.

**Impact**: New file `internal/adapters/http/ratelimit.go`.

---

### D-08: Rate limiter cleanup goroutine uses context for shutdown

**Decision**: `RateLimiter` has a `StartCleanup(ctx context.Context, interval time.Duration)` method that launches a goroutine. Every `interval` (default 2x window), it iterates the map under lock and deletes entries where `now - windowStart > 2*window`. The goroutine exits when `ctx.Done()` fires.

**Rationale**: Without cleanup, the map grows unboundedly with unique IPs (REQ-RATE-05). The cleanup goroutine pattern matches the existing `Service.StartExpirationSweep` at `service.go:423-436`. Using context for lifecycle management is the established codebase pattern.

**Alternatives rejected**:
- Lazy cleanup on each request — Adds latency to every request. Under low traffic, stale entries persist indefinitely.
- `sync.Map` with TTL — No built-in TTL in stdlib; would still need a cleanup goroutine.

**Impact**: Same file `internal/adapters/http/ratelimit.go`.

---

### D-09: Rate limiter integrated as middleware wrapper, not per-handler

**Decision**: The rate limiter exposes a `Middleware(next http.Handler) http.Handler` method. In `server.go`, the rate limiter wraps specific routes:

```go
s.mux.Handle("POST /rooms", roomLimiter.Middleware(http.HandlerFunc(s.createRoomHandler)))
s.mux.Handle("GET /ws/{roomID}", wsLimiter.Middleware(http.HandlerFunc(s.wsHandler)))
```

Two separate `RateLimiter` instances with different limits (rooms: `RATE_LIMIT_ROOMS`, default 10/min; WS: `RATE_LIMIT_WS`, default 20/min).

**Rationale**: The current route registration (`server.go:66-71`) uses `HandleFunc`. Wrapping individual routes (rather than the entire mux) gives per-endpoint control as required by REQ-RATE-01/02. Two instances means independent buckets per endpoint.

**Alternatives rejected**:
- Single limiter for all routes — Can't have different limits per endpoint.
- Global middleware wrapping `s.mux` — Would rate-limit `/health` and other non-sensitive endpoints.

**Impact**: Modified `internal/adapters/http/server.go` (route registration), `RateLimiter` receives limit config from `Server` constructor.

---

### D-10: IP extraction uses RemoteAddr with X-Forwarded-For fallback

**Decision**: The rate limiter extracts the client IP using: (1) `X-Forwarded-For` header first element if present, (2) `X-Real-IP` header if present, (3) `r.RemoteAddr` (strip port). This is needed because Caddy sits in front and sets forwarding headers.

**Rationale**: Behind Caddy, `r.RemoteAddr` is always the Docker-internal IP of Caddy (e.g., `172.18.0.3`). Without header inspection, ALL clients share the same bucket — making rate limiting useless. Caddy automatically sets `X-Forwarded-For`.

**Tradeoff**: `X-Forwarded-For` can be spoofed. Since Caddy is the only entry point and it overwrites the header, this is safe in our deployment topology. Document this assumption in the code.

**Impact**: Helper function `clientIP(r *http.Request) string` in `ratelimit.go`.

---

### D-11: Error events use existing NotifySession with structured codes

**Decision**: Pipeline errors (codec, translation, ICE) are sent to clients via the existing `driven.EventNotifier.NotifySession(sessionID, msgType, fields)` mechanism (`internal/ports/driven/event_notifier.go:5-9`). The `msgType` is always `"error"`. The `fields` map includes `"code"` (machine-readable) and `"message"` (human-readable). The `"session_id"` is added to `fields` when available.

Error codes:
- `"ice-failed"` — ICE connection state reached Failed
- `"translation"` — OpenAI API error during translation
- `"codec"` — Opus encode/decode failure
- `"rate-limited"` — HTTP 429 (this one goes via HTTP response, not WebSocket)

**Rationale**: The `NotifySession` signature (`hub.go:233-258`) already accepts arbitrary `map[string]string` fields. The pipeline already calls `s.notifier.NotifySession(half.sourceSessID, "error", map[string]string{"reason": ...})` at `pipeline.go:163`. We extend this pattern with `"code"` for machine parsing and keep `"reason"` as `"message"` for human display.

**Alternatives rejected**:
- New `ErrorNotifier` port — Over-abstraction for adding a single field to an existing map.
- Typed error struct on the wire — The WebSocket protocol already uses JSON maps; adding a Go struct just for serialization adds complexity without benefit.

**Impact**: Modified `internal/app/roomsvc/pipeline.go` (error notification calls), new ICE state watcher in `internal/adapters/webrtc/pion_peer.go`.

---

### D-12: ICE failure detection via PeerConnection.OnICEConnectionStateChange

**Decision**: In `PionPeer.CreateSession` (`pion_peer.go:63-138`), register an `OnICEConnectionStateChange` callback. When the state transitions to `Failed`, look up the session's event notifier (passed at construction or via a callback registered during session creation) and send the `ice-failed` error event.

**Implementation challenge**: `PionPeer` currently has no reference to `EventNotifier` — it's a driven adapter, not the service. The cleanest approach: add an optional `OnICEFailed func(sessionID string)` callback field to `PionPeer` that `main.go` wires after constructing both `PionPeer` and `Hub`. The callback calls `hub.NotifySession(sessionID, "error", map[string]string{"code": "ice-failed", "message": "ICE connection failed"})`.

**Rationale**: This avoids coupling `PionPeer` to `EventNotifier` (which would create a circular dependency concern). The callback pattern is lightweight and testable.

**Alternatives rejected**:
- Make `PionPeer` depend on `EventNotifier` — Introduces coupling between two driven adapters.
- Polling `ConnectionState` from the service — Adds complexity and latency; callbacks are the standard Pion pattern.

**Impact**: Modified `internal/adapters/webrtc/pion_peer.go` (new callback field + registration in `CreateSession`).

---

### D-13: Feedback endpoint is a pure HTTP handler, no new ports

**Decision**: `POST /feedback` is a handler method on `Server` in `internal/adapters/http/server.go`. It accepts JSON `{session_id, rating, comment}`, validates, and logs via `slog.Info`. No new driven port, no persistence layer.

**Validation rules**:
- `session_id`: required, non-empty string
- `rating`: required, integer 1-5 inclusive
- `comment`: optional string, max 1000 chars (prevent log spam)

**Response**: `200 OK` with `{"status": "ok"}` on success. `400 Bad Request` with descriptive error on validation failure.

**Rationale**: At 5 users, structured logs are sufficient for feedback collection. Adding a database or file persistence is premature. The log can be searched with `jq` or `grep`. The handler follows the same pattern as `createRoomHandler` (`server.go:113-146`).

**Impact**: New handler in `internal/adapters/http/server.go`, new route in `registerRoutes`.

---

### D-14: Health check extended with runtime environment info

**Decision**: `GET /health` response changes from `{"status": "ok"}` to:
```json
{
  "status": "ok",
  "turn_configured": true,
  "api_key_present": true,
  "codec_mode": "opus"
}
```

The `Server` struct receives these values via its `Config` at construction time (set by `main.go`). The health handler reads them from the struct — it does NOT read env vars at request time.

**Rationale**: Operators need quick verification that the deployment is correctly configured. Exposing boolean flags (never the actual credentials) is standard practice. Reading from struct fields (not env vars) means the handler is testable without env manipulation.

**Impact**: Modified `internal/adapters/http/server.go` (`Config` struct, `healthHandler`).

---

### D-16: Dominio gratuito via sslip.io — sin registro ni costo

**Decision**: Usar `sslip.io` como proveedor de DNS gratuito para el alpha. El dominio se construye automáticamente a partir de la IP pública del VPS: `<ip-con-guiones>.sslip.io`. Ejemplo: IP `45.123.45.67` → dominio `45-123-45-67.sslip.io`.

**Rationale**: Firebase Hosting no soporta WebSockets long-lived a servidores externos ni expone los puertos UDP requeridos por Coturn (3478, 49152-65535). Comprar un dominio es innecesario para un alpha de 5 usuarios. `sslip.io` resuelve cualquier IP pública sin registro, es compatible con Let's Encrypt (Caddy obtiene el certificado automáticamente), y no requiere configuración DNS manual.

**Alternatives rejected**:
- Firebase Hosting domain (`*.web.app`) — No soporta WebSocket proxy a VPS externo ni puertos UDP de TURN.
- Dominio comprado — Costo y tiempo innecesarios para 5 usuarios. Puede adquirirse en Sprint 6 si el alpha es exitoso.
- `nip.io` — Alternativa válida, pero `sslip.io` tiene mejor uptime y soporta HTTPS nativo.

**Impact**: `Caddyfile` y `coturn.conf` usan `<IP>.sslip.io` como hostname. El cliente React Native conecta a `wss://<IP>.sslip.io/ws/`. La IP pública del VPS se conoce después del aprovisionamiento.

---

### D-15: loadConfig() centralizes all environment variable reading in main.go

**Decision**: Add a `loadConfig() (appConfig, error)` function in `cmd/server/main.go` that reads and validates all env vars, returning a typed struct. If `OPENAI_API_KEY` is missing, return an error (server exits). All other vars have sensible defaults.

**Config struct**:
```go
type appConfig struct {
    Port             string  // PORT, default "8080"
    LogLevel         string  // LOG_LEVEL, default "info"
    CodecMode        string  // CODEC_MODE, default "opus"
    OpenAIAPIKey     string  // OPENAI_API_KEY, required
    TurnURLs         string  // TURN_URLS, default ""
    TurnUsername     string  // TURN_USERNAME, default ""
    TurnPassword     string  // TURN_PASSWORD, default ""
    RateLimitRooms   int     // RATE_LIMIT_ROOMS, default 10
    RateLimitWS      int     // RATE_LIMIT_WS, default 20
}
```

**Rationale**: Currently env vars are read inline (`os.Getenv("OPENAI_API_KEY")` at `main.go:71`). Centralizing reads makes the startup sequence testable and self-documenting. The pattern follows 12-factor app principles.

**Impact**: Modified `cmd/server/main.go`.

---

## Component Design

### OpusCodec Adapter

**File**: `internal/adapters/codec/opus.go`

```go
package codec

import (
    "context"
    "github.com/pion/opus"
    "github.com/Sergiotsk/TalkGo/internal/ports/driven"
)

var _ driven.AudioCodec = (*OpusCodec)(nil)

const (
    opusSampleRate = 24000
    opusChannels   = 1
    opusFrameSize  = 480 // 20ms at 24kHz
)

type OpusCodec struct {
    sampleRate int
    channels   int
    frameSize  int
}

func NewOpusCodec() *OpusCodec {
    return &OpusCodec{
        sampleRate: opusSampleRate,
        channels:   opusChannels,
        frameSize:  opusFrameSize,
    }
}
```

**Decode flow**: Spawns a goroutine. For each `[]byte` frame from `opusIn`:
1. Create a `pion/opus.Decoder` (lazy, one per goroutine — decoders are not thread-safe)
2. Decode Opus frame to PCM16 `[]int16`
3. Convert `[]int16` to `[]byte` (little-endian, 2 bytes per sample)
4. Send to output channel

**Encode flow**: Spawns a goroutine. For each `[]byte` PCM16 frame from `pcmIn`:
1. Validate frame length is even (each sample = 2 bytes)
2. Convert `[]byte` to `[]int16`
3. Encode to Opus via `pion/opus.Encoder`
4. Send to output channel

**Error handling**:
- Decode/encode errors are logged (`slog.Error`) and the frame is skipped (best-effort)
- Context cancellation closes the output channel and exits the goroutine
- Encoder/decoder are created lazily inside the goroutine (one per stream, not shared)

---

### Rate Limiter Middleware

**File**: `internal/adapters/http/ratelimit.go`

```go
type bucket struct {
    count       int
    windowStart time.Time
}

type RateLimiter struct {
    mu      sync.Mutex
    buckets map[string]*bucket
    limit   int           // max requests per window
    window  time.Duration // window size (e.g., 1 minute)
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter
func (rl *RateLimiter) Allow(ip string) (bool, int)  // returns (allowed, retryAfterSec)
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler
func (rl *RateLimiter) StartCleanup(ctx context.Context, interval time.Duration)
```

**`Allow` algorithm**:
```
lock mutex
get or create bucket for IP
if now - bucket.windowStart > window:
    reset count to 0, set windowStart to now
if count >= limit:
    retryAfter = window - (now - windowStart)
    unlock, return (false, retryAfter in seconds)
count++
unlock, return (true, 0)
```

**`Middleware` behavior on rejection**:
```
w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
writeJSON(w, 429, {"error": "rate-limited", "retry_after_seconds": retryAfter})
```

**Cleanup goroutine**: Runs every `2 * window`. Iterates all buckets, deletes entries where `now - windowStart > 2 * window`.

**IP extraction** (`clientIP` function):
```go
func clientIP(r *http.Request) string {
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        // Take the first IP (client's real IP, set by Caddy)
        if i := strings.IndexByte(xff, ','); i > 0 {
            return strings.TrimSpace(xff[:i])
        }
        return strings.TrimSpace(xff)
    }
    if xri := r.Header.Get("X-Real-IP"); xri != "" {
        return xri
    }
    host, _, _ := net.SplitHostPort(r.RemoteAddr)
    return host
}
```

---

### Error Event System

**Flow**: Errors originate in 3 places and reach the client via the same path:

```
Pipeline (codec/translation error)     PionPeer (ICE failed)
          |                                    |
          v                                    v
  s.notifier.NotifySession(             onICEFailed callback
    sessionID,                          -> hub.NotifySession(
    "error",                               sessionID,
    map[string]string{                     "error",
      "code": "translation",              map[string]string{
      "message": "...",                     "code": "ice-failed",
      "session_id": sessionID,              "message": "...",
    })                                      "session_id": sessionID,
          |                                })
          v                                    |
    Hub.NotifySession                          v
          |                             Hub.NotifySession
          v                                    |
    client.send <- JSON                        v
          |                             client.send <- JSON
          v
    WebSocket TextMessage to client
```

**Wire format** (all error events):
```json
{
  "type": "error",
  "code": "ice-failed|translation|codec",
  "message": "Human-readable description",
  "session_id": "uuid-if-available"
}
```

The `session_id` field is omitted from JSON when empty (via `omitempty` in the `fields` map — the `NotifySession` already uses `json.Marshal` on a `map[string]string` at `hub.go:245`).

**Pipeline changes** (`pipeline.go`): Replace bare `"reason"` field with `"code"` + `"message"` + `"session_id"`:

Current (line 163):
```go
s.notifier.NotifySession(half.sourceSessID, "error",
    map[string]string{"reason": fmt.Sprintf("audio track setup failed: %v", err)})
```

New:
```go
s.notifier.NotifySession(half.sourceSessID, "error",
    map[string]string{
        "code":       "codec",
        "message":    "audio processing error",
        "session_id": half.sourceSessID,
    })
```

---

### Environment Configuration

| Env Var | Type | Default | Required | Validation |
|---------|------|---------|----------|------------|
| `PORT` | string | `"8080"` | No | Must be valid port (1-65535) |
| `LOG_LEVEL` | string | `"info"` | No | One of: debug, info, warn, error |
| `CODEC_MODE` | string | `"opus"` | No | One of: opus, passthrough |
| `OPENAI_API_KEY` | string | — | **Yes** | Non-empty |
| `TURN_URLS` | string | `""` | No | Comma-separated TURN URLs |
| `TURN_USERNAME` | string | `""` | No | Used only if TURN_URLS is set |
| `TURN_PASSWORD` | string | `""` | No | Used only if TURN_URLS is set |
| `RATE_LIMIT_ROOMS` | int | `10` | No | >= 0 (0 = disabled) |
| `RATE_LIMIT_WS` | int | `20` | No | >= 0 (0 = disabled) |

**Validation behavior**:
- `OPENAI_API_KEY` missing/empty: `slog.Error("openai_api_key_required", ...)` + `os.Exit(1)`
- `CODEC_MODE` unknown value: `slog.Error("invalid_codec_mode", ...)` + `os.Exit(1)`
- `RATE_LIMIT_*` parse failure: log warning, use default
- `PORT` invalid: log error, exit

**Startup log** (emitted once after config load):
```json
{"msg": "config_loaded", "component": "main", "port": "8080", "codec_mode": "opus", "turn_configured": true, "rate_limit_rooms": 10, "rate_limit_ws": 20}
```

---

### Feedback Handler

**Route**: `POST /feedback` added to `registerRoutes()` in `server.go`.

**Request struct**:
```go
type feedbackRequest struct {
    SessionID string `json:"session_id"`
    Rating    int    `json:"rating"`
    Comment   string `json:"comment"`
}
```

**Handler logic**:
```go
func (s *Server) feedbackHandler(w http.ResponseWriter, r *http.Request) {
    var req feedbackRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, 400, "invalid request body")
        return
    }
    if req.SessionID == "" {
        writeError(w, 400, "session_id is required")
        return
    }
    if req.Rating < 1 || req.Rating > 5 {
        writeError(w, 400, "rating must be between 1 and 5")
        return
    }
    if len(req.Comment) > 1000 {
        req.Comment = req.Comment[:1000] // truncate, don't reject
    }

    slog.Info("feedback_received",
        "component", "http",
        "session_id", req.SessionID,
        "rating", req.Rating,
        "comment", req.Comment,
    )
    writeJSON(w, 200, map[string]string{"status": "ok"})
}
```

---

### TURN Configuration

**File**: `internal/adapters/webrtc/config.go` (new file, extracted from `pion_peer.go`)

```go
// BuildICEConfig creates a Config with STUN servers and optional TURN servers.
// If turnURLs is empty, returns STUN-only config (identical to DefaultConfig).
func BuildICEConfig(turnURLs, turnUser, turnPass string) Config {
    servers := []pionwebrtc.ICEServer{
        {URLs: []string{"stun:stun.l.google.com:19302"}},
    }

    if turnURLs != "" {
        urls := strings.Split(turnURLs, ",")
        for i := range urls {
            urls[i] = strings.TrimSpace(urls[i])
        }
        servers = append(servers, pionwebrtc.ICEServer{
            URLs:       urls,
            Username:   turnUser,
            Credential: turnPass,
        })
    }

    return Config{ICEServers: servers}
}
```

**main.go wiring**:
```go
// Replace: peer := webrtcadapter.NewPionPeer(webrtcadapter.DefaultConfig())
// With:
webrtcCfg := webrtcadapter.BuildICEConfig(cfg.TurnURLs, cfg.TurnUsername, cfg.TurnPassword)
peer := webrtcadapter.NewPionPeer(webrtcCfg)
```

`DefaultConfig()` remains unchanged for backward compatibility — existing tests can still use it.

---

### Health Check Extension

**Modified** `healthHandler` in `server.go`:

The `Config` struct gains three new fields:
```go
type Config struct {
    // ... existing fields ...
    TurnConfigured bool
    APIKeyPresent  bool
    CodecMode      string
}
```

Set by `main.go` during `Server` construction. The handler reads these fields:

```go
func (s *Server) healthHandler(w http.ResponseWriter, _ *http.Request) {
    writeJSON(w, http.StatusOK, map[string]any{
        "status":          "ok",
        "turn_configured": s.cfg.TurnConfigured,
        "api_key_present": s.cfg.APIKeyPresent,
        "codec_mode":      s.cfg.CodecMode,
    })
}
```

Note: the response type changes from `map[string]string` to `map[string]any` to support boolean values.

---

## File Changes Map

| File | What Changes | Why |
|------|-------------|-----|
| `internal/adapters/codec/opus_codec.go` | **Rename** to `passthrough.go` | File name matches content (REQ-COD-04) |
| `internal/adapters/codec/opus_codec_test.go` | **Rename** to `passthrough_test.go` | Follow source file rename |
| `internal/adapters/http/server.go` | Add feedback handler, extend health check, wire rate limiter middleware to routes, add `Config` fields | REQ-UX-05/06, REQ-OPS-06, REQ-RATE-01/02 |
| `internal/adapters/webrtc/pion_peer.go` | Add `OnICEFailed` callback field, register `OnICEConnectionStateChange` in `CreateSession` | REQ-UX-01 |
| `internal/app/roomsvc/pipeline.go` | Update `notifier.NotifySession` calls to include `"code"`, `"message"`, `"session_id"` fields | REQ-UX-02/03/07 |
| `cmd/server/main.go` | Add `loadConfig()`, codec selection switch, TURN wiring, rate limiter init, wire ICE failure callback, validate `OPENAI_API_KEY` | REQ-COD-03, REQ-NET-02, REQ-OPS-03, REQ-RATE-03 |
| `Makefile` | Add `docker-build`, `docker-up`, `docker-down` targets | REQ-OPS convenience |
| `go.mod` | Add `github.com/pion/opus` | REQ-COD-01 |

---

## New Files

| File | Contents |
|------|----------|
| `internal/adapters/codec/opus.go` | `OpusCodec` struct implementing `driven.AudioCodec` with `pion/opus` |
| `internal/adapters/codec/opus_test.go` | Unit tests: decode Opus->PCM16, encode PCM16->Opus, round-trip, context cancellation |
| `internal/adapters/codec/passthrough.go` | **Renamed** from `opus_codec.go` (no content change) |
| `internal/adapters/codec/passthrough_test.go` | **Renamed** from `opus_codec_test.go` (no content change) |
| `internal/adapters/webrtc/config.go` | `BuildICEConfig()` function |
| `internal/adapters/webrtc/config_test.go` | Unit tests: STUN-only, TURN additive, multiple URLs |
| `internal/adapters/http/ratelimit.go` | `RateLimiter` struct, `Allow`, `Middleware`, `StartCleanup`, `clientIP` |
| `internal/adapters/http/ratelimit_test.go` | Unit tests: allow/deny, cleanup, multiple IPs, disabled (limit=0) |
| `Dockerfile` | Multi-stage Go build → scratch |
| `docker-compose.yml` | Three services: talkgo, coturn, caddy |
| `Caddyfile` | Reverse proxy config with WebSocket support |
| `deploy/coturn/turnserver.conf` | Coturn long-term credential config |
| `.env.example` | Template for required env vars |
| `docs/deploy/vps-setup.md` | VPS deployment guide |
| `docs/deploy/expo-go-guide.md` | Tester guide for Expo Go |

---

## Ops Artifacts (non-Go)

### Dockerfile

```dockerfile
# === Stage 1: Build ===
FROM golang:1.23-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /talkgo ./cmd/server

# === Stage 2: Runtime ===
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /talkgo /talkgo

EXPOSE 8080

ENTRYPOINT ["/talkgo"]
```

**Key decisions**:
- `CGO_ENABLED=0` — Required for `scratch` base (no libc). `pion/opus` is pure Go, so no CGO needed.
- `-ldflags="-s -w"` — Strips debug info and DWARF symbols, reducing binary size by ~30%.
- `ca-certificates.crt` — Copied from builder for HTTPS calls to OpenAI API.
- `scratch` base — Zero attack surface, smallest possible image. The Go binary is ~10-15MB after stripping.
- `EXPOSE 8080` — Documents the internal port (Caddy connects to this).

---

### docker-compose.yml

```yaml
version: "3.8"

services:
  talkgo:
    build: .
    restart: unless-stopped
    ports:
      - "8080:8080"
    env_file:
      - .env
    depends_on:
      - coturn
    networks:
      - talkgo-net

  coturn:
    image: coturn/coturn:latest
    restart: unless-stopped
    ports:
      - "3478:3478/udp"
      - "3478:3478/tcp"
      - "49152-49200:49152-49200/udp"
    volumes:
      - ./deploy/coturn/turnserver.conf:/etc/turnserver.conf:ro
    command: ["-c", "/etc/turnserver.conf"]
    networks:
      - talkgo-net

  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    depends_on:
      - talkgo
    networks:
      - talkgo-net

volumes:
  caddy_data:
  caddy_config:

networks:
  talkgo-net:
    driver: bridge
```

**Key decisions**:
- Port range `49152-49200` for Coturn relay ports (49 ports, sufficient for 5 concurrent users).
- `caddy_data` volume persists Let's Encrypt certificates across container restarts.
- All services on `talkgo-net` — they communicate by service name (e.g., `talkgo:8080`).
- `env_file: .env` — Secrets (`OPENAI_API_KEY`, `TURN_PASSWORD`) never in the compose file.
- Coturn config mounted read-only.

---

### Caddyfile

```caddyfile
{
    email {$ACME_EMAIL:admin@example.com}
}

{$DOMAIN:localhost} {
    # Reverse proxy all traffic to the Go server.
    # Caddy automatically handles WebSocket upgrade headers.
    reverse_proxy talkgo:8080

    # Logging
    log {
        output stdout
        format json
    }
}
```

**Key decisions**:
- `{$DOMAIN}` is set via environment variable — configurable per deployment.
- Caddy handles WebSocket upgrades transparently — no special `@websocket` matcher needed for basic proxying. Caddy's `reverse_proxy` natively supports WebSocket upgrade.
- `{$ACME_EMAIL}` for Let's Encrypt registration — required for automatic HTTPS.
- HTTP -> HTTPS redirect is automatic in Caddy when a domain is configured (non-localhost).
- JSON log format matches the Go server's structured logging.

---

### Coturn Configuration

**File**: `deploy/coturn/turnserver.conf`

```conf
# TalkGo Coturn configuration
# Long-term credentials for alpha deployment (5 users)

# Network
listening-port=3478
min-port=49152
max-port=49200
fingerprint

# Authentication — long-term credentials
lt-cred-mech
realm=talkgo.example.com
user=talkgo:changeme-turn-password

# Security
no-multicast-peers
no-cli
no-tlsv1
no-tlsv1_1

# Logging
log-file=stdout
verbose
```

**Key decisions**:
- `lt-cred-mech` — Long-term credential mechanism. At 5 users, static credentials are acceptable.
- `user=talkgo:changeme-turn-password` — Must match `TURN_USERNAME` and `TURN_PASSWORD` env vars in `.env`. The `.env.example` documents this.
- `min-port=49152, max-port=49200` — 49 relay ports. Each TURN allocation uses one port. 5 concurrent users need at most 10 allocations (2 per session). 49 is generous headroom.
- `no-cli` — Disables the Coturn CLI interface (security hardening).
- `log-file=stdout` — Logs to Docker stdout for `docker compose logs` access.

---

### .env.example

```env
# === Required ===
OPENAI_API_KEY=sk-your-key-here

# === Server ===
PORT=8080
LOG_LEVEL=info
CODEC_MODE=opus

# === TURN (Coturn) ===
# Must match deploy/coturn/turnserver.conf user/password
TURN_URLS=turn:coturn:3478
TURN_USERNAME=talkgo
TURN_PASSWORD=changeme-turn-password

# === Rate Limiting ===
# Requests per minute per IP. Set to 0 to disable.
RATE_LIMIT_ROOMS=10
RATE_LIMIT_WS=20

# === Caddy ===
DOMAIN=talkgo.example.com
ACME_EMAIL=admin@example.com
```

---

## Test Strategy

### OpusCodec (`internal/adapters/codec/opus_test.go`)

| Test | What | How |
|------|------|-----|
| `TestOpusCodec_Decode_ValidFrame` | Decode known Opus frame to PCM16 | Generate Opus frame with encoder in test setup, decode, verify output length > 0 and even byte count |
| `TestOpusCodec_Encode_ValidPCM` | Encode PCM16 to Opus | Generate 480-sample PCM16 silence, encode, verify output > 0 |
| `TestOpusCodec_RoundTrip` | Encode then decode preserves non-zero audio | Encode 440Hz tone PCM16, decode result, verify not all zeros |
| `TestOpusCodec_Encode_OddLength_Error` | Reject incomplete samples | Send odd-length `[]byte`, verify error or skip (design choice: skip frame) |
| `TestOpusCodec_CancelContext_ClosesOutput` | Context cancellation cleanup | Cancel context, verify output channel closes, no goroutine leak |
| `TestOpusCodec_ClosedInput_ClosesOutput` | Input close propagates | Close input channel, verify output channel closes |
| `TestOpusCodec_InterfaceCompliance` | Compile-time check | `var _ driven.AudioCodec = (*OpusCodec)(nil)` |

### TURN Config (`internal/adapters/webrtc/config_test.go`)

| Test | What | How |
|------|------|-----|
| `TestBuildICEConfig_STUNOnly` | Empty TURN_URLS = STUN-only | Call with `""`, verify 1 ICEServer with STUN URL |
| `TestBuildICEConfig_WithTURN` | TURN added alongside STUN | Call with `"turn:srv:3478"`, verify 2 ICEServers |
| `TestBuildICEConfig_MultipleURLs` | Comma-separated URLs | Call with `"turn:a:3478,turn:b:3478"`, verify single TURN ICEServer with 2 URLs |
| `TestBuildICEConfig_Credentials` | Username/password set | Verify ICEServer.Username and Credential match inputs |

### Rate Limiter (`internal/adapters/http/ratelimit_test.go`)

| Test | What | How |
|------|------|-----|
| `TestRateLimiter_Allow_UnderLimit` | Requests below limit pass | Call `Allow` N times (N < limit), all return true |
| `TestRateLimiter_Allow_AtLimit` | N+1th request rejected | Call `Allow` limit+1 times, last returns false |
| `TestRateLimiter_Allow_WindowReset` | Window resets after duration | Fill bucket, advance time past window, verify next request allowed |
| `TestRateLimiter_Allow_IndependentIPs` | Separate buckets per IP | Fill one IP, verify another IP still allowed |
| `TestRateLimiter_Allow_Disabled` | Limit=0 disables rate limiting | Create with limit=0, verify all requests allowed |
| `TestRateLimiter_Cleanup_RemovesStale` | Stale entries deleted | Add entries, advance time, run cleanup, verify map size |
| `TestRateLimiter_RetryAfter` | Correct retry-after seconds | Fill bucket at t=0, check retryAfter = window - elapsed |
| `TestRateLimiter_Middleware_429Response` | HTTP 429 format correct | Use httptest, verify status + Retry-After header + body JSON |

### Feedback Handler (`internal/adapters/http/server_test.go`)

| Test | What | How |
|------|------|-----|
| `TestFeedbackHandler_Valid` | Accept valid feedback | POST with valid JSON, verify 200 + `{"status":"ok"}` |
| `TestFeedbackHandler_MissingSessionID` | Reject missing session_id | POST without session_id, verify 400 |
| `TestFeedbackHandler_InvalidRating` | Reject out-of-range rating | rating=0, rating=6, verify 400 |
| `TestFeedbackHandler_InvalidJSON` | Reject malformed body | POST with `{bad`, verify 400 |
| `TestFeedbackHandler_EmptyBody` | Reject empty body | POST with empty, verify 400 |

### Health Check (`internal/adapters/http/server_test.go`)

| Test | What | How |
|------|------|-----|
| `TestHealthHandler_IncludesTurnAndAPIKey` | Extended fields present | GET /health, verify JSON has `turn_configured`, `api_key_present`, `codec_mode` |

### Error Events (existing test files)

| Test | Location | What |
|------|----------|------|
| ICE failed notification | `internal/adapters/webrtc/pion_peer_test.go` | Simulate ICE state change to Failed, verify callback fires |
| Translation error notification | `internal/app/roomsvc/pipeline_internal_test.go` | Mock translator returns error, verify NotifySession called with `code: "translation"` |
| Codec error notification | `internal/app/roomsvc/pipeline_internal_test.go` | Mock codec returns error, verify NotifySession called with `code: "codec"` |

---

## Dependency Changes

**go.mod additions**:
```
require github.com/pion/opus v0.0.1 // (use latest available version)
```

This is the ONLY new dependency. All other Sprint 5 features use stdlib (`sync`, `time`, `net`, `net/http`, `strings`, `strconv`, `encoding/json`, `log/slog`).

**No dependency changes for**:
- Rate limiting (stdlib `sync.Mutex` + `time`)
- Feedback endpoint (stdlib `net/http` + `encoding/json`)
- TURN config (Pion already in `go.mod` — `pion/webrtc/v3`, `pion/ice/v2`, `pion/turn/v2`)
- Error events (existing `EventNotifier` interface)
- Docker/Caddy/Coturn (ops artifacts, not Go code)

---

## Migration Notes

### Breaking Changes: None

All existing behavior is preserved when env vars are absent:
- `CODEC_MODE` defaults to `"opus"` (new behavior, but PassthroughCodec was never correct for production)
- `TURN_URLS` empty = STUN-only (current behavior)
- `RATE_LIMIT_*` defaults = rate limiting active (new behavior, but non-breaking)
- `GET /health` response gains new fields (additive, not breaking)

### File Renames

The `opus_codec.go` → `passthrough.go` rename requires updating the test file name. No import paths change (same package `codec`). Git tracks the rename cleanly.

### Deployment Sequence

1. Build Docker image: `docker build -t talkgo .`
2. Copy `.env.example` to `.env`, fill in real values
3. DNS: point domain to VPS IP
4. `docker compose up -d`
5. Verify: `curl https://domain/health`
6. Share Expo Go link with testers
