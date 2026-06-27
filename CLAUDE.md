# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is TalkGo

Real-time audio translation platform for face-to-face multilingual conversations. Two users speaking different languages join a room; each hears the other's speech translated into their own language in real time.

## Common Commands

```bash
# Backend
make run            # go run ./cmd/server
make test           # go test -race -cover ./...
make test-verbose   # go test -race -v -cover ./...
make lint           # golangci-lint run ./... (depguard enforced)
make fmt            # gofmt + goimports
make check          # fmt + lint + test (CI gate)
make cover          # HTML coverage report

# Single package test
go test -race ./internal/app/roomsvc/...

# Load generator
go run ./cmd/loadgen -server ws://localhost:8080 -room abc123 -duration 30s -sessions 10

# Mobile
cd mobile && npm start        # Metro bundler
cd mobile && npm run android
cd mobile && npm run ios
cd mobile && npm test         # Jest
cd mobile && npm run lint     # ESLint
```

## Environment Variables

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `OPENAI_API_KEY` | YES | — | Translation via OpenAI Realtime API |
| `PORT` | NO | 8080 | Listen port |
| `LOG_LEVEL` | NO | info | debug/info/warn/error |
| `CODEC_MODE` | NO | opus | `opus` or `passthrough` |
| `TURN_URLS` | NO | — | TURN server URLs (comma-separated) |
| `TURN_USERNAME` / `TURN_PASSWORD` | NO | — | TURN auth |
| `RATE_LIMIT_ROOMS` | NO | 10 | Rooms per IP per hour |
| `RATE_LIMIT_WS` | NO | 20 | WebSocket connections per IP |

Copy `.env.example` as `.env` for local dev.

## Architecture

**Hexagonal Architecture** with strict layer isolation enforced by `depguard` in CI.

```
cmd/server (wiring)
    └── internal/adapters/  (concrete implementations)
            └── internal/app/roomsvc/  (orchestration / service layer)
                    └── internal/ports/  (interfaces only)
                            └── internal/domain/  (pure business logic)
```

Dependency direction flows **inward only**:
```
adapters → app → ports ← domain
```

### Layer Rules (violations fail CI)

- **`internal/domain/`** — zero infrastructure imports. No `net/http`, `pion/*`, `database/*`, no `context`. Only pure Go types and standard math/strings.
- **`internal/ports/`** — interfaces only, no implementations. Driving ports (`ports/driving/`) are entry points (RoomManager, SignalingHandler). Driven ports (`ports/driven/`) are dependencies (WebRTCPeer, Translator, AudioCodec, etc.).
- **`internal/app/roomsvc/`** — orchestrates domain + ports. Houses `Service` (implements RoomManager + SignalingHandler), `Pipeline` (runHalf goroutines per room), `LatencyTracker`.
- **`internal/adapters/`** — thin wrappers: `codec/` (Opus/Passthrough), `webrtc/` (Pion), `translator/` (OpenAI Realtime), `signaling/` (WebSocket hub), `http/` (REST + rate limiting), `tts/`.
- **`cmd/server`** — dependency injection only: wires adapters → service → HTTP server.

Adding a new dependency requires a doc in `docs/adr/`.

### Audio Translation Pipeline

Per room, two mirrored `runHalf` goroutines (A→B and B→A):

1. **OnAudioTrack** — capture Opus RTP from WebRTC peer
2. **Decode** — Opus → PCM (`codec.Decode`)
3. **Translate** — PCM → OpenAI Realtime API → PCM in target language (`translator.TranslateStream`)
4. **Encode** — PCM → Opus (`codec.Encode`)
5. **Send** — Opus → target peer (`peer.SendAudio`)

Each stage is timed by `LatencyTracker` and emitted as structured JSON log event `chunk_latency`.

### Session Lifecycle

`POST /rooms` → WebSocket `/ws?room=&session=` → WebRTC offer/answer exchange via signaling hub → both peers connected → pipeline starts → pipeline emits `session_event: pipeline_start` → audio flows → peer disconnects → `session_event: pipeline_stop`.

## Go Conventions

**Error handling:**
```go
return fmt.Errorf("creating room: %w", err)   // always wrap with context
var ErrRoomFull = errors.New("room is full")   // sentinel errors in domain
```

**Constructors:**
```go
func NewXxx(deps...) (*Xxx, error) { /* validate, return err if invalid */ }
```

**Context:** All I/O functions take `context.Context` as first param. Domain logic does NOT use context.

**Docs:** All exported symbols need a doc comment starting with the symbol name.

## Testing

Strict TDD — write the test first. Every port interface has a mock.

Coverage targets: `domain/` ≥ 80%, `app/` ≥ 70%, `adapters/` ≥ 60%.

Table-driven tests with `t.Run()`. Test files colocated (`foo.go` → `foo_test.go`).

The `-race` flag is always on in `make test`. Never remove it.

## Logging

Structured JSON via `log/slog`. Key events:

| msg | When |
|-----|------|
| `session_event` (session_start/end) | User connects/disconnects |
| `session_event` (pipeline_start/stop) | Both peers ready / one leaves |
| `chunk_latency` | Per audio chunk (stages: capture, decode, translate, encode, send) |
| `session_error` | Adapter errors with count and stage |

```bash
# Filter logs with jq
go run ./cmd/server | jq 'select(.msg == "chunk_latency") | {total_ms, stages}'
go run ./cmd/server | jq 'select(.level == "error")'
```

## Mobile (React Native / Expo)

- **State:** Zustand (`src/store/userStore.ts`)
- **WebRTC:** `react-native-webrtc` + custom hooks in `src/hooks/` (`useSignaling`, `useWebRTC`, `useReconnection`)
- **Path alias:** `@/*` → `src/*`
- **Platform splits:** `useAudioSession.ios.ts`, `useForegroundService.android.ts`
- **Screens:** `OnboardingScreen` → `HomeScreen` → `ConversationScreen`

## Deployment

```bash
make docker-build   # multi-stage Docker image (CGO + libopus)
make docker-up      # talkgo + coturn + caddy
make docker-down
```

Docker notes: CGO is required for the Opus codec — the builder image installs `libopus-dev`. The runtime image links against `libopus0`.
