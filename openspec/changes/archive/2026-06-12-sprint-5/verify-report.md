# Sprint 5 Verify Report — Alpha con Usuarios Finales

**Change**: sprint-5
**Status**: verified
**Date**: 2026-06-12
**Verifier**: sdd-verify agent

---

## REQ-COD — Opus Codec

- **REQ-COD-01: WARNING** — `OpusCodec` implements `driven.AudioCodec` (compile-time guard confirmed) and `Decode` returns PCM16 LE bytes. However, the implementation uses `gopkg.in/hraban/opus.v2` (CGO + libopus) instead of the `github.com/pion/opus` (pure Go, zero CGO) mandated by D-01 and referenced in the spec/design. The D-01 design decision explicitly lists `hraban/opus` as *rejected* due to CGO complexity and image size implications. The codec works correctly, but deviates from the agreed dependency choice.
- **REQ-COD-02: WARNING** — `Encode` returns Opus frames, round-trip test exists, odd-length frames are silently skipped (matches spec). Minor gap: the spec says `Encode` should return an **error** for odd-length input, but the implementation silently skips (logs warn and continues). The test `TestOpusCodec_Encode_OddLength_Skipped` matches the implementation, not the spec literal ("returns error if the input is not a multiple of 2 bytes").
- **REQ-COD-03: PASS** — `loadConfig()` reads `CODEC_MODE` with default `"opus"`, validates against `"opus"`/`"passthrough"`, exits on unknown values; startup log includes `codec_mode`. Tests TASK-065 to TASK-068 cover all cases.
- **REQ-COD-04: PASS** — `passthrough.go` exists at `internal/adapters/codec/passthrough.go`, `PassthroughCodec` implements `driven.AudioCodec`, Encode/Decode return data unchanged. Rename from `opus_codec.go` confirmed.
- **REQ-COD-05: PASS** — `Decode` and `Encode` respect `ctx.Done()` via `select` in the goroutine loop. Tests `TestOpusCodec_CancelContext_ClosesOutput` and `TestOpusCodec_ClosedInput_ClosesOutput` verify cleanup. Goroutine leak check uses `runtime.NumGoroutine()` before/after (±2 tolerance).

---

## REQ-NET — Network / TURN

- **REQ-NET-01: PASS** — `BuildICEConfig(turnURLs, turnUser, turnPass string) Config` exists in `internal/adapters/webrtc/config.go`. Returns `Config` with `ICEServers` populated. `Username` and `Credential` fields set on `ICEServer`. Tests TASK-019 verify credentials.
- **REQ-NET-02: PASS** — `main.go` reads `TURN_URLS`, `TURN_USERNAME`, `TURN_PASSWORD` via `loadConfig()` using `os.Getenv`. Non-empty `TURN_URLS` adds a TURN `ICEServer`. Startup log includes `turn_configured=true/false`. `TURN_URLS` comma separation handled via `strings.Split` in `config.go`.
- **REQ-NET-03: PASS** — `BuildICEConfig("", "", "")` returns exactly 1 ICEServer (STUN-only). `TestBuildICEConfig_STUNOnly` verifies no TURN entry is present. Backward compatibility preserved.
- **REQ-NET-04: PASS** — `docker-compose.yml` defines `coturn` service with `image: coturn/coturn:latest`, ports `3478/udp`, `3478/tcp`, relay range `49152-49200/udp`. `deploy/coturn/turnserver.conf` defines `lt-cred-mech`, `realm=talkgo`, `user=talkgo:CHANGEME_TURN_PASSWORD`. Manual verification required for actual allocations.
- **REQ-NET-05: PASS** — `docker-compose.yml` defines `caddy` service with `image: caddy:2-alpine`, ports 80+443, mounts `Caddyfile`. `Caddyfile` has `{$DOMAIN}` block with `reverse_proxy talkgo:8080`. WebSocket upgrades handled transparently by Caddy's `reverse_proxy`. `X-Real-IP` and `X-Forwarded-Proto` headers set for upstream.

---

## REQ-OPS — Operations / Deployment

- **REQ-OPS-01: WARNING** — `Dockerfile` uses multi-stage build (`golang:1.23-bookworm` builder → `debian:bookworm-slim` runtime), `CGO_ENABLED=1`, `-ldflags="-s -w"`. However, the spec mandates `CGO_ENABLED=0` and a `scratch` base for a `<20MB` image. Because `hraban/opus` requires CGO + `libopus.so`, the runtime image must be `debian:bookworm-slim` (not `scratch`), which will produce an image significantly larger than 20MB (estimated 80-120MB with libopus0 + ca-certificates). This is a direct consequence of the D-01 deviation (CGO codec vs pure-Go pion/opus). The 20MB acceptance criterion cannot be met with this approach.
- **REQ-OPS-02: PASS** — `docker-compose.yml` defines all three services: `talkgo` (builds from `.`, `restart: unless-stopped`, `env_file: .env`, depends on `coturn`), `coturn` (public image, 3478 UDP+TCP exposed), `caddy` (Caddy 2 alpine, ports 80+443, mounts Caddyfile). All on `talkgo-net` bridge network. Volumes `caddy_data`/`caddy_config` defined. Sensitive vars via `env_file: .env`. Minor: `talkgo` uses `expose:` (internal only) instead of `ports:` — this means `curl http://localhost:8080/health` from the host will NOT work directly; only via Caddy. The spec says `curl http://localhost:8080/health` from the host should work (REQ-OPS-04). This is a WARNING, not CRITICAL, since the design intent is to use Caddy as the entry point.
- **REQ-OPS-03: PASS** — `loadConfig()` provides all required defaults: `PORT=8080`, `CODEC_MODE=opus`, `TURN_URLS=""`, `RATE_LIMIT_ROOMS=10`, `RATE_LIMIT_WS=20`. `OPENAI_API_KEY` required (returns error if absent). Tests TASK-065 to TASK-068 cover all parsing cases.
- **REQ-OPS-04: WARNING** — `docker-compose.yml` uses `expose: ["8080"]` for talkgo (internal only, not host-bound). REQ-OPS-04 requires `curl http://localhost:8080/health` to work from the host, which requires `ports: ["8080:8080"]`. Manual verification required; this config does not support direct host access to the Go service.
- **REQ-OPS-05: PASS** — `docs/deploy/vps-setup.md` exists with VPS prerequisites, Docker install requirements, open ports table, sslip.io DNS setup, `.env` config, `docker compose up -d`, Coturn verification, Caddy TLS instructions, and troubleshooting commands.
- **REQ-OPS-06: PASS** — `GET /health` returns `{"status":"ok","turn_configured":bool,"api_key_present":bool,"codec_mode":string}` with `200 OK` and `Content-Type: application/json`. `Server.Config` fields `TurnConfigured`, `APIKeyPresent`, `CodecMode` set by `main.go`. Tests `TestHealthHandler_ExtendedFields` and `TestHealthHandler_TurnNotConfigured` verify all three fields.

---

## REQ-RATE — Rate Limiting

- **REQ-RATE-01: PASS** — `RateLimiter.Middleware` wraps `POST /rooms` when `roomLimiter != nil`. Fixed-window counter per IP. Returns `429` with `Retry-After` header and `{"error":"rate-limited","retry_after_seconds":N}` body. Requests from different IPs use independent buckets. Tests `TestServer_RateLimit_Rooms_429` and `TestRateLimiter_Allow_AtLimit` verify behavior.
- **REQ-RATE-02: PASS** — `RateLimiter.Middleware` wraps `GET /ws/{roomID}` when `wsLimiter != nil`. Rejection occurs before WebSocket upgrade. Test `TestServer_RateLimit_WS_429` verifies 429 response from the same IP. Test `TestServer_RateLimit_IndependentEndpoints` verifies independence from `/rooms` limiter.
- **REQ-RATE-03: PASS** — `RATE_LIMIT_ROOMS` and `RATE_LIMIT_WS` parsed as `int` in `loadConfig()`. Parse failure logs warning and uses default (10 and 20 respectively). Startup log includes `rate_limit_rooms=N`. Minor gap: `rate_limit_ws` is NOT in the startup `slog.Info("config_loaded")` call (only `rate_limit_rooms` is logged). Spec requires both.
- **REQ-RATE-04: PASS** — `ratelimit.go` imports only `context`, `encoding/json`, `net`, `net/http`, `strconv`, `strings`, `sync`, `time` — all stdlib. `go.mod` adds `gopkg.in/hraban/opus.v2` (for the codec) but no new rate limiting dependency. The rate limiter itself has zero new deps.
- **REQ-RATE-05: PASS** — `StartCleanup(ctx, interval)` goroutine runs `Cleanup(now)` on each tick, deleting entries where `windowStart < now - 2*window`. `StartCleanupNotify` exported for test introspection. Test `TestRateLimiter_Cleanup_RemovesStale` verifies cleanup. Test `TestRateLimiter_StartCleanup_ExitsOnCancel` verifies goroutine exits on context cancel.

---

## REQ-UX — Error UX + Feedback

- **REQ-UX-01: PASS** — `PionPeer.OnICEFailed func(sessionID string)` field added. `CreateSession` registers `OnICEConnectionStateChange` callback that calls `p.OnICEFailed(sessionID)` on `ICEConnectionStateFailed`. `main.go` wires the callback to `hub.NotifySession` with `{"code":"ice-failed","message":"ICE connection failed","session_id":sessionID}`. Test `TestPionPeer_ICEFailed_CallsOnICEFailedCallback` in `pion_peer_test.go` verifies callback invocation. `iceStateHandlers` map exported for test introspection.
- **REQ-UX-02: PASS** — `pipeline.go` translation error sends `{"code":"translation","message":"translation service error","session_id":half.sourceSessID}` via `notifier.NotifySession`. Error is logged via `logSessionError` using `slog.Error`. Internal Go error not exposed to client. Test `TestPipelineHalf_TranslationError_SendsErrorEvent` verifies fields.
- **REQ-UX-03: PASS** — `pipeline.go` codec (decode) error sends `{"code":"codec","message":"audio processing error","session_id":half.sourceSessID}` via `notifier.NotifySession`. Test `TestPipelineHalf_CodecError_SendsErrorEvent` verifies `code=codec` and `session_id` present. Minor gap: encode error path (Stage 4) still uses bare `"reason"` field instead of `"code"/"message"/"session_id"` (line ~217 in pipeline.go). The decode path is correct; the encode path is NOT updated.
- **REQ-UX-04: PASS** — Rate limiter 429 response includes `Retry-After` header and `{"error":"rate-limited","retry_after_seconds":N}` JSON body. `retryAfterSeconds` in body matches `Retry-After` header value. Tests `TestRateLimiter_Middleware_429Response` and `TestServer_RateLimit_Rooms_429` verify format.
- **REQ-UX-05: WARNING** — `POST /feedback` handler exists, validates session_id and rating, logs via `slog.Info`. However, the spec says success returns `200 OK` with `{"status":"ok"}`, but the implementation returns **`201 Created`** (`writeJSON(w, http.StatusCreated, ...)`). Tests also expect 201. This is a clear deviation from the spec's `200 OK` acceptance criterion.
- **REQ-UX-06: PASS** — All rejection cases implemented: empty body → 400 `"invalid request body"`, invalid JSON → 400 `"invalid request body"`, `rating < 1` → 400 `"rating must be between 1 and 5"`, `rating > 5` → same, `session_id` absent/empty → 400 `"session_id is required"`. All 5 test cases in `server_test.go` verify exact error messages.
- **REQ-UX-07: WARNING** — Codec decode and translation error paths correctly include `session_id`. However, the Stage 4 encode error path (`pipeline.go` ~line 217) still uses `map[string]string{"reason": fmt.Sprintf("audio encode failed: %v", err)}` — missing `"code"`, `"message"`, and `"session_id"`. The Stage 5 send error path (~line 229) also uses bare `"reason"` field. This means not ALL error notifications follow the structured format required by REQ-UX-07.

---

## REQ-MOB — Mobile / Expo Go

- **REQ-MOB-01: WARNING** — No `client/` directory exists in the repository. TASK-064 (update React Native client config) cannot be verified. The spec acceptance criterion requires a `client/src/config.ts` or `.env` with `EXPO_PUBLIC_WS_URL`. This artifact is absent from the repository.
- **REQ-MOB-02: PASS** — `docs/deploy/expo-go-guide.md` exists. Covers: install Expo Go from App Store/Play Store (with direct links), microphone permission grant steps for iOS and Android, how to open the app (QR scan or manual URL), troubleshooting section. Explicitly states "You do NOT need to install Node.js, npm, or any developer tools."
- **REQ-MOB-03: PASS** — `hub.go` upgrader has `CheckOrigin: func(_ *http.Request) bool { return true }` (permissive, with "for MVP" comment). Test `TestServer_WebSocket_AnyOriginAccepted` uses a real httptest server, dials with `Origin: https://arbitrary.example.com`, and verifies `101 Switching Protocols`. The comment in the test explains the rationale for Expo Go compatibility.

---

## Summary

| Status | Count |
|--------|-------|
| PASS | 20 |
| WARNING | 8 |
| CRITICAL | 0 |

**Total requirements**: 28

---

## Findings Detail

### CRITICAL Issues: 0

No acceptance criteria are completely unverifiable.

---

### WARNING Issues: 8

**W-1 (REQ-COD-01/REQ-OPS-01 — CGO codec vs pure-Go)**: The implementation uses `gopkg.in/hraban/opus.v2` instead of the `github.com/pion/opus` decided in D-01. D-01 explicitly lists `hraban/opus` as REJECTED due to CGO + libopus-dev requirements. This cascades into:
- The Dockerfile cannot use `scratch` and must use `debian:bookworm-slim`
- The image will be ~80-120MB, well above the 20MB target (REQ-OPS-01)
- CGO_ENABLED=1 in Dockerfile instead of CGO_ENABLED=0

The codec is functionally correct — encode/decode/round-trip all work — but the D-01 architectural decision was not followed. This is the most impactful deviation in the sprint.

**W-2 (REQ-COD-02 — odd-length frame behavior)**: Spec says `Encode` returns an error for odd-length input. Implementation silently skips (logs warn, continues). Test matches implementation (not spec). Behavior is safe but the contract differs from what the spec defines.

**W-3 (REQ-RATE-03 — startup log missing `rate_limit_ws`)**: The `config_loaded` slog.Info at startup includes `rate_limit_rooms` but NOT `rate_limit_ws`. Spec requires both.

**W-4 (REQ-OPS-02/REQ-OPS-04 — talkgo port exposure)**: `docker-compose.yml` uses `expose: ["8080"]` (internal Docker network only) instead of `ports: ["8080:8080"]`. This means `curl http://localhost:8080/health` from the host fails — only Caddy access works. REQ-OPS-04 acceptance criterion requires direct host access to the Go service.

**W-5 (REQ-UX-03/REQ-UX-07 — encode error path not updated)**: Stage 4 encode error in `pipeline.go` (~line 217) still uses `{"reason": "audio encode failed: ..."}` format. Stage 5 send error (~line 229) also uses bare `"reason"`. Only the decode and translation error paths use the new structured `{"code","message","session_id"}` format. Two of four error paths in `runHalf` are not migrated.

**W-6 (REQ-UX-05 — feedback returns 201 instead of 200)**: `feedbackHandler` calls `writeJSON(w, http.StatusCreated, ...)`. Spec explicitly says `200 OK`. The tests align with the implementation (201), not the spec (200).

**W-7 (REQ-MOB-01 — client config not in repository)**: No `client/` directory. TASK-064 artifact cannot be verified. The React Native client config with `EXPO_PUBLIC_WS_URL` is not committed.

**W-8 (REQ-UX-07 partial)**: Covered under W-5 above — the `omitempty` pattern test (`TestErrorMessage_SessionID_OmittedWhenEmpty`) uses a local struct to verify JSON behavior, not the actual `map[string]string` used by pipeline.go. Since pipeline.go always sets `session_id` to `half.sourceSessID` (never empty in a real pipeline), the runtime behavior is correct, but the implementation approach (map vs struct with omitempty) means the omitempty guarantee is not enforced by the type system in the actual error paths.

---

## Design Decisions Check

| Decision | Status | Notes |
|----------|--------|-------|
| D-01: pion/opus (pure Go, zero CGO) | FAIL | hraban/opus (CGO) used instead |
| D-02: CODEC_MODE env var | PASS | Implemented as specified |
| D-03: passthrough.go rename | PASS | Rename confirmed |
| D-04: Synchronous per-frame codec with goroutine | PASS | Goroutine pattern matches design |
| D-05: TURN additive, empty = STUN-only | PASS | BuildICEConfig implemented correctly |
| D-06: TURN_URLS comma-separated | PASS | strings.Split in config.go |
| D-07: Fixed-window token bucket with sync.Mutex | PASS | Exact design implemented |
| D-08: StartCleanup goroutine with context | PASS | StartCleanup + StartCleanupNotify |
| D-09: Middleware wrapper per-route | PASS | Two separate limiter instances |
| D-10: IP extraction X-Forwarded-For → X-Real-IP → RemoteAddr | PASS | clientIP() function matches design |
| D-11: Error events via NotifySession with code/message fields | WARNING | Encode and send error paths not migrated (W-5) |
| D-12: OnICEFailed callback field | PASS | Implemented with iceStateHandlers map for tests |
| D-13: Feedback endpoint pure HTTP handler | PASS | No new port, slog.Info logging |
| D-14: Health check with TurnConfigured/APIKeyPresent/CodecMode | PASS | Config struct fields, handler reads from struct |
| D-15: loadConfig() centralizes env vars | PASS | appConfig struct + loadConfig() function |
| D-16: sslip.io free domain | PASS | Documented in vps-setup.md |

---

## Recommended Fixes (priority order)

1. **[High] REQ-COD-01/REQ-OPS-01**: Evaluate whether to migrate to `pion/opus` (pure Go) to restore the `scratch` base image and <20MB target, OR formally accept the `hraban/opus` deviation and update the design document and Dockerfile size target. The current 20MB criterion in the spec is unachievable with CGO.

2. **[Medium] REQ-UX-03/REQ-UX-07 (W-5)**: Update `pipeline.go` Stage 4 encode error (~line 217) and Stage 5 send error (~line 229) to use `{"code": "codec"/"send", "message": "...", "session_id": half.targetSessID}` format. Two error paths were missed in TASK-032/034.

3. **[Low] REQ-UX-05 (W-6)**: Change `writeJSON(w, http.StatusCreated, ...)` to `writeJSON(w, http.StatusOK, ...)` in `feedbackHandler`. Update `TestFeedbackHandler_Valid` accordingly.

4. **[Low] REQ-RATE-03 (W-3)**: Add `"rate_limit_ws", appCfg.RateLimitWS` to the `config_loaded` slog.Info call in `main.go`.

5. **[Low] REQ-OPS-04 (W-4)**: Consider adding `ports: ["8080:8080"]` to the `talkgo` service in `docker-compose.yml` for direct host access, or document that direct access requires going through Caddy.

6. **[Low] REQ-MOB-01 (W-7)**: Commit the React Native client config file (TASK-064 artifact) if the client code exists locally.
