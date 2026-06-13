# Sprint 5 Tasks — Alpha con Usuarios Finales

**Change**: sprint-5
**Status**: tasks
**Date**: 2026-06-12
**Total tasks**: 72

---

## Sequencing Notes

- **TDD order is strict**: every `(test)` task precedes its paired `(impl)` task.
- **Phase 1 (rename) must complete before Phase 2** — `passthrough_test.go` is the renamed file that
  already covers REQ-COD-04; no new passthrough tests are written.
- **Phase 5 (error events) depends on Phase 2** — `OpusCodec` must exist before codec-error path is wired.
- **Phase 11 (main.go wiring) is last Go phase** — it integrates everything built in phases 2–8.
- **Phase 9 (ops artifacts) and Phase 10 (docs) are independent** — can run in parallel with phases 5–8.
- `(ops)` tasks are mechanical file operations: rename, `go mod tidy`, file creation with no paired test.

---

## Phase 1: Setup & Rename

> Rename the misleadingly-named `opus_codec.go` so the new `opus.go` can own that name.
> No logic changes. Existing tests pass without modification.

- [x] TASK-001 (ops): Rename `internal/adapters/codec/opus_codec.go` → `passthrough.go` (git mv) — covers REQ-COD-04
- [x] TASK-002 (ops): Rename `internal/adapters/codec/opus_codec_test.go` → `passthrough_test.go` (git mv) — covers REQ-COD-04
- [x] TASK-003 (ops): Run `go test ./internal/adapters/codec/...` — verify all existing PassthroughCodec tests still pass after rename — covers REQ-COD-04
- [x] TASK-004 (ops): Add `github.com/pion/opus` to `go.mod` via `go get github.com/pion/opus` and run `go mod tidy` — covers REQ-COD-01

---

## Phase 2: OpusCodec

> Implement the real Opus codec adapter using pion/opus (pure Go, zero CGO).
> Tests come first in every pair — no impl task is touched until its test task is checked off.

- [x] TASK-005 (test): Write `TestOpusCodec_InterfaceCompliance` in `internal/adapters/codec/opus_test.go` — compile-time guard `var _ driven.AudioCodec = (*OpusCodec)(nil)` — covers REQ-COD-01, REQ-COD-02
- [x] TASK-006 (impl): Create `internal/adapters/codec/opus.go` with `OpusCodec` struct, constants (`opusSampleRate=24000`, `opusChannels=1`, `opusFrameSize=480`), and `NewOpusCodec()` constructor satisfying the interface guard — covers REQ-COD-01, REQ-COD-02
- [x] TASK-007 (test): Write `TestOpusCodec_Decode_ValidFrame` — generate a real Opus frame with a pion encoder in test setup, send through `Decode`, verify output `len > 0` and byte count is even (PCM16 = 2 bytes/sample) — covers REQ-COD-01
- [x] TASK-008 (impl): Implement `OpusCodec.Decode(ctx, opusIn)` — spawn goroutine; per frame: create lazy `pion/opus.Decoder` (24kHz mono), decode Opus→`[]int16`, marshal to `[]byte` little-endian, send to output channel; exit on ctx cancel or closed input — covers REQ-COD-01
- [x] TASK-009 (test): Write `TestOpusCodec_Encode_ValidPCM` — generate 480 PCM16 silence samples (960 bytes), send through `Encode`, verify output `len > 0` — covers REQ-COD-02
- [x] TASK-010 (impl): Implement `OpusCodec.Encode(ctx, pcmIn)` — spawn goroutine; per frame: validate even length (skip frame + log warn on odd), unmarshal `[]byte`→`[]int16`, encode via `pion/opus.Encoder` (AppVoIP), send result; exit on ctx cancel or closed input — covers REQ-COD-02
- [x] TASK-011 (test): Write `TestOpusCodec_Encode_OddLength_Skipped` — send a 3-byte (odd) frame through `Encode`, verify no output frame is produced (frame is silently skipped) and output channel eventually closes — covers REQ-COD-02
- [x] TASK-012 (test): Write `TestOpusCodec_RoundTrip` — generate 440Hz tone as PCM16, encode→decode, verify resulting PCM is not all zeros (audio survived the round-trip) — covers REQ-COD-01, REQ-COD-02
- [x] TASK-013 (test): Write `TestOpusCodec_CancelContext_ClosesOutput` — cancel context before sending any frame; verify output channel is closed; use `runtime.NumGoroutine()` before/after to confirm no goroutine leak — covers REQ-COD-05
- [x] TASK-014 (test): Write `TestOpusCodec_ClosedInput_ClosesOutput` — close input channel immediately; verify output channel closes — covers REQ-COD-05
- [x] TASK-015 (test): Write `TestOpusCodec_Encode_Silence_NoError` — send 960 bytes of zeros through `Encode`, verify output frame is produced (silence encodes without error) — covers REQ-COD-02

---

## Phase 3: TURN Configuration

> Add `BuildICEConfig()` in a new `config.go` file. `DefaultConfig()` and `PionPeer` are untouched.
> Tests verify the function's output shape, not Pion internals.

- [x] TASK-016 (test): Write `TestBuildICEConfig_STUNOnly` in `internal/adapters/webrtc/config_test.go` — call `BuildICEConfig("", "", "")`, assert exactly 1 ICEServer with STUN URL, no TURN entry — covers REQ-NET-03
- [x] TASK-017 (test): Write `TestBuildICEConfig_WithTURN` — call `BuildICEConfig("turn:srv:3478", "user", "pass")`, assert 2 ICEServers (STUN + TURN), TURN entry has correct URL — covers REQ-NET-01, REQ-NET-02
- [x] TASK-018 (test): Write `TestBuildICEConfig_MultipleURLs` — call with `"turn:a:3478,turn:b:3478"`, assert TURN ICEServer has 2 URLs in its `URLs` slice — covers REQ-NET-01 (D-06 comma-separation)
- [x] TASK-019 (test): Write `TestBuildICEConfig_Credentials` — verify `ICEServer.Username` and `Credential` match the `turnUser` and `turnPass` inputs — covers REQ-NET-01
- [x] TASK-020 (impl): Create `internal/adapters/webrtc/config.go` with `BuildICEConfig(turnURLs, turnUser, turnPass string) Config` — STUN-only when `turnURLs == ""`, otherwise appends TURN `ICEServer` with `strings.Split`-parsed URLs — covers REQ-NET-01, REQ-NET-02, REQ-NET-03

---

## Phase 4: Rate Limiter

> New file `ratelimit.go` with fixed-window counter. Tests exercise the `Allow` logic directly,
> then middleware integration via `httptest`.

- [x] TASK-021 (test): Write `TestRateLimiter_Allow_UnderLimit` in `internal/adapters/http/ratelimit_test.go` — call `Allow` N times (N < limit), all must return `(true, 0)` — covers REQ-RATE-01, REQ-RATE-02
- [x] TASK-022 (test): Write `TestRateLimiter_Allow_AtLimit` — call `Allow` `limit+1` times for same IP, last call must return `(false, retryAfter > 0)` — covers REQ-RATE-01, REQ-RATE-02, REQ-UX-04
- [x] TASK-023 (test): Write `TestRateLimiter_Allow_IndependentIPs` — fill bucket for IP-A, verify IP-B is still allowed — covers REQ-RATE-01
- [x] TASK-024 (test): Write `TestRateLimiter_Allow_Disabled` — create limiter with `limit=0`, call `Allow` 100 times, all must return `(true, 0)` — covers REQ-RATE-03
- [x] TASK-025 (test): Write `TestRateLimiter_RetryAfter_Correct` — fill bucket at t=0, call `Allow` again 10s into a 60s window, verify `retryAfter` ≈ 50s (within 2s tolerance) — covers REQ-UX-04, REQ-RATE-01
- [x] TASK-026 (test): Write `TestRateLimiter_Cleanup_RemovesStale` — add entries, call cleanup with `now + 3*window`, verify `len(rl.buckets) == 0` — covers REQ-RATE-05
- [x] TASK-027 (test): Write `TestRateLimiter_Allow_WindowReset` — fill bucket, simulate time past window end (inject a past `windowStart` directly into bucket), call `Allow`, verify it is allowed (window reset) — covers REQ-RATE-01
- [x] TASK-028 (test): Write `TestRateLimiter_Middleware_429Response` in `ratelimit_test.go` — use `httptest.NewRecorder`, exhaust limit, verify next response is `429`, has `Retry-After` header, and body is `{"error":"rate-limited","retry_after_seconds":N}` — covers REQ-RATE-01, REQ-RATE-02, REQ-UX-04
- [x] TASK-029 (impl): Create `internal/adapters/http/ratelimit.go` with:
  - `bucket` struct (`count int`, `windowStart time.Time`)
  - `RateLimiter` struct (`mu sync.Mutex`, `buckets map[string]*bucket`, `limit int`, `window time.Duration`)
  - `NewRateLimiter(limit int, window time.Duration) *RateLimiter`
  - `clientIP(r *http.Request) string` — X-Forwarded-For → X-Real-IP → RemoteAddr fallback
  - `Allow(ip string) (bool, int)` — fixed-window algorithm; returns `(allowed, retryAfterSec)`
  - `Middleware(next http.Handler) http.Handler` — calls `Allow`, writes 429 JSON on reject
  - `StartCleanup(ctx context.Context, interval time.Duration)` — goroutine deletes entries where `now - windowStart > 2*window`
  — covers REQ-RATE-01, REQ-RATE-02, REQ-RATE-04, REQ-RATE-05
- [x] TASK-030 (test): Write `TestRateLimiter_StartCleanup_ExitsOnCancel` — start cleanup goroutine, cancel context, verify goroutine exits (use `runtime.NumGoroutine` before/after or a done channel) — covers REQ-RATE-05

---

## Phase 5: Error Events

> Extend existing pipeline error paths and add ICE failure callback.
> Tests verify the shape of `NotifySession` calls via mock `EventNotifier`.

- [x] TASK-031 (test): Write `TestPipelineHalf_CodecError_SendsErrorEvent` in `internal/app/roomsvc/pipeline_internal_test.go` — inject mock `AudioCodec` that returns error in `Decode`; verify `EventNotifier.NotifySession` is called with `msgType="error"`, `fields["code"]="codec"`, `fields["session_id"]=sourceSessID` — covers REQ-UX-03, REQ-UX-07
- [x] TASK-032 (impl): Update `pipeline.go` codec error notification — replace bare `"reason"` field with `"code": "codec"`, `"message": "audio processing error"`, `"session_id": half.sourceSessID` in the `notifier.NotifySession` call — covers REQ-UX-03, REQ-UX-07
- [x] TASK-033 (test): Write `TestPipelineHalf_TranslationError_SendsErrorEvent` in `pipeline_internal_test.go` — inject mock `Translator` that returns error; verify `NotifySession` called with `fields["code"]="translation"`, `fields["session_id"]` present — covers REQ-UX-02, REQ-UX-07
- [x] TASK-034 (impl): Update `pipeline.go` translation error notification — replace bare error fields with `"code": "translation"`, `"message": "translation service error"`, `"session_id": half.sourceSessID` — covers REQ-UX-02, REQ-UX-07
- [x] TASK-035 (test): Write `TestPionPeer_ICEFailed_CallsOnICEFailedCallback` in `internal/adapters/webrtc/pion_peer_test.go` — create `PionPeer` with `OnICEFailed` callback set; simulate `ICEConnectionStateFailed` transition; verify callback is invoked with the correct `sessionID` — covers REQ-UX-01
- [x] TASK-036 (impl): Add `OnICEFailed func(sessionID string)` field to `PionPeer` struct; in `CreateSession`, register `OnICEConnectionStateChange` callback that calls `p.OnICEFailed(sessionID)` when state transitions to `Failed` — covers REQ-UX-01
- [x] TASK-037 (test): Write `TestErrorMessage_SessionID_OmittedWhenEmpty` — construct a `map[string]string` error payload with `session_id=""` and marshal to JSON; verify `session_id` key is absent from output (testing the omitempty pattern) — covers REQ-UX-07

---

## Phase 6: Feedback Endpoint

> Pure HTTP handler — no new port, no persistence. Tests cover validation table exhaustively.

- [x] TASK-038 (test): Write `TestFeedbackHandler_Valid` in `internal/adapters/http/server_test.go` — POST valid JSON `{"session_id":"s1","rating":3,"comment":"good"}`, verify `200 OK` + body `{"status":"ok"}` — covers REQ-UX-05
- [x] TASK-039 (test): Write `TestFeedbackHandler_MissingSessionID` — POST without `session_id`, verify `400` + `{"error":"session_id is required"}` — covers REQ-UX-06
- [x] TASK-040 (test): Write `TestFeedbackHandler_EmptySessionID` — POST with `session_id:""`, verify `400` + same error — covers REQ-UX-06
- [x] TASK-041 (test): Write `TestFeedbackHandler_RatingTooLow` — POST with `rating:0`, verify `400` + `{"error":"rating must be between 1 and 5"}` — covers REQ-UX-06
- [x] TASK-042 (test): Write `TestFeedbackHandler_RatingTooHigh` — POST with `rating:6`, verify `400` + same error — covers REQ-UX-06
- [x] TASK-043 (test): Write `TestFeedbackHandler_InvalidJSON` — POST `{bad`, verify `400` + `{"error":"invalid request body"}` — covers REQ-UX-06
- [x] TASK-044 (test): Write `TestFeedbackHandler_EmptyBody` — POST with no body, verify `400` + `{"error":"invalid request body"}` — covers REQ-UX-06
- [x] TASK-045 (impl): Add `feedbackRequest` struct and `feedbackHandler` method to `internal/adapters/http/server.go` — validate `session_id` non-empty, `rating` in [1,5], truncate `comment` at 1000 chars, log via `slog.Info`, return `{"status":"ok"}` — covers REQ-UX-05, REQ-UX-06
- [x] TASK-046 (impl): Register `POST /feedback` route in `registerRoutes()` in `server.go` — covers REQ-UX-05

---

## Phase 7: Health Check Extension

> Extend `GET /health` response with TURN and API key presence flags.
> Config fields are set at construction time — handler reads struct, not env vars.

- [x] TASK-047 (test): Write `TestHealthHandler_ExtendedFields` in `internal/adapters/http/server_test.go` — construct `Server` with `Config{TurnConfigured: true, APIKeyPresent: true, CodecMode: "opus"}`, GET `/health`, verify JSON body contains `"turn_configured":true`, `"api_key_present":true`, `"codec_mode":"opus"`, `"status":"ok"` — covers REQ-OPS-06
- [x] TASK-048 (test): Write `TestHealthHandler_TurnNotConfigured` — construct with `TurnConfigured: false, APIKeyPresent: false`, verify `"turn_configured":false`, `"api_key_present":false` — covers REQ-OPS-06
- [x] TASK-049 (impl): Add `TurnConfigured bool`, `APIKeyPresent bool`, `CodecMode string` fields to `httpserver.Config` struct in `server.go` — covers REQ-OPS-06
- [x] TASK-050 (impl): Update `healthHandler` in `server.go` — return `map[string]any{"status":"ok","turn_configured":s.cfg.TurnConfigured,"api_key_present":s.cfg.APIKeyPresent,"codec_mode":s.cfg.CodecMode}` (change from `map[string]string` to `map[string]any`) — covers REQ-OPS-06

---

## Phase 8: Rate Limiter Integration into Server Routes

> Wire rate limiter middleware onto `POST /rooms` and `GET /ws/{roomID}`.
> Server constructor receives two pre-built limiters.

- [x] TASK-051 (test): Write `TestServer_RateLimit_Rooms_429` in `server_test.go` — use `httptest`, exhaust rooms limiter (limit=1 in test), verify next POST `/rooms` returns `429` with `Retry-After` header — covers REQ-RATE-01, REQ-UX-04
- [x] TASK-052 (test): Write `TestServer_RateLimit_WS_429` — exhaust WS limiter (limit=1), verify next GET `/ws/testroom` returns `429` before upgrade — covers REQ-RATE-02, REQ-UX-04
- [x] TASK-053 (test): Write `TestServer_RateLimit_IndependentEndpoints` — exhaust rooms limiter, verify WS endpoint is still accessible (separate bucket) — covers REQ-RATE-01, REQ-RATE-02
- [x] TASK-054 (impl): Update `NewServer` signature in `server.go` to accept `roomLimiter *RateLimiter` and `wsLimiter *RateLimiter` — covers REQ-RATE-01, REQ-RATE-02
- [x] TASK-055 (impl): Update `registerRoutes()` in `server.go` — wrap `POST /rooms` with `roomLimiter.Middleware(...)` and `GET /ws/{roomID}` with `wsLimiter.Middleware(...)` using `s.mux.Handle` (not `HandleFunc`) — covers REQ-RATE-01, REQ-RATE-02, REQ-NET-01 (design D-09)

---

## Phase 9: Ops Artifacts

> Non-Go files. No test-first. Each task creates one artifact file.

- [x] TASK-056 (ops): Create `Dockerfile` at repo root — multi-stage: `golang:1.23-bookworm` builder with `CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /talkgo ./cmd/server` + libopus-dev, runtime stage from `debian:bookworm-slim` with libopus0 + ca-certificates, `EXPOSE 8080`, `ENTRYPOINT ["/talkgo"]` — covers REQ-OPS-01
- [x] TASK-057 (ops): Create `docker-compose.yml` at repo root — three services: `talkgo` (builds from `.`, `restart: unless-stopped`, `env_file: .env`), `coturn` (image `coturn/coturn:latest`, ports `3478` UDP+TCP, relay range `49152-49200/udp`, mounts `turnserver.conf`), `caddy` (image `caddy:2-alpine`, ports `80`+`443`, mounts `Caddyfile`, volumes for certs); all on `talkgo-net` bridge network — covers REQ-OPS-02, REQ-NET-04, REQ-NET-05
- [x] TASK-058 (ops): Create `Caddyfile` at repo root — `{$DOMAIN}` block with `reverse_proxy talkgo:8080`, JSON log output, `{$ACME_EMAIL}` for ACME — covers REQ-NET-05
- [x] TASK-059 (ops): Create `deploy/coturn/turnserver.conf` — `listening-port=3478`, `tls-listening-port=5349`, `min-port=49152`, `max-port=49200`, `fingerprint`, `lt-cred-mech`, `realm=talkgo`, `user=talkgo:CHANGEME_TURN_PASSWORD` (with comment to replace manually), `no-multicast-peers`, `no-cli`, `log-file=stdout`, `verbose` — covers REQ-NET-04
- [x] TASK-060 (ops): Create `.env.example` at repo root — all env vars documented with descriptions, required vs optional marked, matching `appConfig` fields — covers REQ-OPS-03
- [x] TASK-061 (ops): Add `docker-build`, `docker-up`, `docker-down`, `docker-logs` targets to `Makefile` — covers REQ-OPS-04

---

## Phase 10: Deployment Docs

> Markdown files only. No test-first. Written for two audiences: VPS operator and Expo Go tester.

- [x] TASK-062 (ops): Create `docs/deploy/vps-setup.md` — covers: VPS prerequisites (Docker 24+, open ports 80/443/3478), DNS setup, clone + `.env` config, `docker compose up -d`, `curl /health` smoke test, Coturn verification with `nc -u`, Caddy TLS verification, log access (`docker compose logs`), service restart — covers REQ-OPS-05
- [x] TASK-063 (ops): Create `docs/deploy/expo-go-guide.md` — covers: install Expo Go (App Store/Play Store), grant microphone permission, scan QR or enter URL manually (`wss://domain/ws/{roomID}`), what to do if connection fails (check mic, try different network, contact admin), NO dev tooling required — covers REQ-MOB-02
- [x] TASK-064 (ops): Update React Native client config (e.g., `client/src/config.ts` or `.env`) — set `WS_URL` via `EXPO_PUBLIC_WS_URL` env var defaulting to `wss://`, remove any hardcoded `localhost` references, document dev vs prod switch — covers REQ-MOB-01

---

## Phase 11: Config Centralization & main.go Wiring

> All env var reads move into `loadConfig()`. All Sprint 5 components are wired together.
> This phase has the most cross-cutting impact — do it last so all components exist.

- [x] TASK-065 (test): Write integration test in `cmd/server/main_test.go` — call `loadConfig()` with `OPENAI_API_KEY` unset, verify it returns an error — covers REQ-OPS-03
- [x] TASK-066 (test): Write test — call `loadConfig()` with `CODEC_MODE=invalid`, verify error returned — covers REQ-COD-03
- [x] TASK-067 (test): Write test — call `loadConfig()` with `RATE_LIMIT_ROOMS=abc` (invalid int), verify default `10` is used (no error, warning logged) — covers REQ-RATE-03
- [x] TASK-068 (test): Write test — call `loadConfig()` with all env vars set to valid values, verify all fields populated correctly including `TurnURLs`, `RateLimitRooms`, `CodecMode` — covers REQ-OPS-03
- [x] TASK-069 (impl): Add `appConfig` struct and `loadConfig() (appConfig, error)` to `cmd/server/main.go` — reads `PORT`, `LOG_LEVEL`, `CODEC_MODE` (default `"opus"`), `OPENAI_API_KEY` (required), `TURN_URLS`, `TURN_USERNAME`, `TURN_PASSWORD`, `RATE_LIMIT_ROOMS` (default `10`), `RATE_LIMIT_WS` (default `20`); validates OPENAI key and CODEC_MODE — covers REQ-OPS-03, REQ-COD-03, REQ-RATE-03
- [x] TASK-070 (impl): Update `run()` in `main.go` — call `loadConfig()`, add `CODEC_MODE` switch to instantiate `OpusCodec` or `PassthroughCodec`, call `webrtcadapter.BuildICEConfig(cfg.TurnURLs, cfg.TurnUsername, cfg.TurnPassword)`, instantiate `RateLimiter`s with env-driven limits, pass `TurnConfigured`/`APIKeyPresent`/`CodecMode` to `httpserver.Config`, emit startup `slog.Info("config_loaded", ...)` log — covers REQ-COD-03, REQ-NET-02, REQ-OPS-03, REQ-RATE-03, REQ-OPS-06
- [x] TASK-071 (impl): Wire `OnICEFailed` callback on `PionPeer` in `main.go` — after constructing `hub` and `peer`, set `peer.OnICEFailed = func(sessionID string) { hub.NotifySession(sessionID, "error", map[string]string{"code": "ice-failed", "message": "ICE connection failed", "session_id": sessionID}) }` — covers REQ-UX-01, REQ-MOB-03
- [x] TASK-072 (test): Write `TestServer_WebSocket_AnyOriginAccepted` in `server_test.go` — make WebSocket upgrade request with `Origin: https://arbitrary.example.com`, verify response is `101 Switching Protocols` not `403 Forbidden`; add code comment in `wsHandler` documenting the permissive origin policy for Expo Go — covers REQ-MOB-03

---

## Phase 12: Final Verification Pass

> No new code. Run all tests, check tool hygiene, verify no regressions.

- [x] TASK-073 (ops): Run `go vet ./...` — zero warnings expected — covers all REQs
- [x] TASK-074 (ops): Run `go test ./... -race -count=1` — all tests green, no data race detected — covers all REQs
- [x] TASK-075 (ops): Run `go mod tidy` — verify `go.mod` has exactly one new `require` entry (`github.com/pion/opus`), no stray deps — covers REQ-RATE-04
- [x] TASK-076 (ops): Inspect `internal/adapters/http/ratelimit.go` imports — confirm only `net`, `net/http`, `sync`, `time`, `strconv`, `strings` from stdlib — covers REQ-RATE-04
- [x] TASK-077 (ops): Manual smoke test — start server with `CODEC_MODE=passthrough OPENAI_API_KEY=fake`, verify log contains `codec_mode=passthrough`, `turn_configured=false` — covers REQ-COD-03, REQ-OPS-03

---

## Task Summary by Phase

| Phase | Tasks | Type breakdown |
|-------|-------|----------------|
| 1 — Setup & Rename | TASK-001 to 004 | 4 ops |
| 2 — OpusCodec | TASK-005 to 015 | 6 test, 2 impl, 3 test |
| 3 — TURN Config | TASK-016 to 020 | 4 test, 1 impl |
| 4 — Rate Limiter | TASK-021 to 030 | 8 test, 2 impl |
| 5 — Error Events | TASK-031 to 037 | 4 test, 3 impl |
| 6 — Feedback Endpoint | TASK-038 to 046 | 7 test, 2 impl |
| 7 — Health Check Extension | TASK-047 to 050 | 2 test, 2 impl |
| 8 — Rate Limiter Integration | TASK-051 to 055 | 3 test, 2 impl |
| 9 — Ops Artifacts | TASK-056 to 061 | 6 ops |
| 10 — Deployment Docs | TASK-062 to 064 | 3 ops |
| 11 — Config & main.go Wiring | TASK-065 to 072 | 4 test, 4 impl |
| 12 — Final Verification | TASK-073 to 077 | 5 ops |
| **Total** | **77** | **38 test, 18 impl, 21 ops** |

---

## REQ Coverage Matrix

| Requirement | Tasks |
|-------------|-------|
| REQ-COD-01 | TASK-004, 005, 006, 007, 008, 012 |
| REQ-COD-02 | TASK-005, 006, 009, 010, 011, 012, 015 |
| REQ-COD-03 | TASK-065, 066, 069, 070 |
| REQ-COD-04 | TASK-001, 002, 003 |
| REQ-COD-05 | TASK-013, 014 |
| REQ-NET-01 | TASK-016, 017, 018, 019, 020 |
| REQ-NET-02 | TASK-017, 020, 070 |
| REQ-NET-03 | TASK-016, 020 |
| REQ-NET-04 | TASK-057, 059 |
| REQ-NET-05 | TASK-057, 058 |
| REQ-OPS-01 | TASK-056 |
| REQ-OPS-02 | TASK-057 |
| REQ-OPS-03 | TASK-060, 065, 067, 068, 069, 070 |
| REQ-OPS-04 | TASK-061 |
| REQ-OPS-05 | TASK-062 |
| REQ-OPS-06 | TASK-047, 048, 049, 050, 070 |
| REQ-RATE-01 | TASK-021, 022, 023, 025, 027, 028, 029, 051, 053, 055 |
| REQ-RATE-02 | TASK-021, 022, 028, 029, 052, 053, 054, 055 |
| REQ-RATE-03 | TASK-024, 067, 068, 069, 070 |
| REQ-RATE-04 | TASK-029, 075, 076 |
| REQ-RATE-05 | TASK-026, 029, 030 |
| REQ-UX-01 | TASK-035, 036, 071 |
| REQ-UX-02 | TASK-033, 034 |
| REQ-UX-03 | TASK-031, 032 |
| REQ-UX-04 | TASK-022, 025, 028, 051, 052 |
| REQ-UX-05 | TASK-038, 045, 046 |
| REQ-UX-06 | TASK-039, 040, 041, 042, 043, 044, 045 |
| REQ-UX-07 | TASK-031, 032, 033, 034, 037 |
| REQ-MOB-01 | TASK-064 |
| REQ-MOB-02 | TASK-063 |
| REQ-MOB-03 | TASK-071, 072 |
