# TalkGo Developer Onboarding

## Project Overview

TalkGo is a real-time audio translation platform designed for face-to-face multilingual conversations using individual mobile devices. Two people speak in their own languages, and the system translates and streams the audio back to each participant in real time.

### Architecture

The backend follows **Hexagonal Architecture (Ports & Adapters)**:

- **Core domain** (`internal/domain/`) — pure Go types with zero external dependencies
- **Port interfaces** (`internal/ports/`) — driving (inbound) and driven (outbound) contracts
- **Application layer** (`internal/app/roomsvc/`) — orchestrates domain logic via ports
- **Adapters** (`internal/adapters/`) — concrete implementations (WebRTC via Pion, OpenAI Realtime, HTTP, WebSocket signaling)

This isolation means domain logic never imports infrastructure packages. Verified by `golangci-lint` with `depguard`.

### Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.23 |
| WebSocket | gorilla/websocket |
| WebRTC | pion/webrtc v3 |
| Translation | OpenAI Realtime API |
| Audio Codec | Passthrough (Opus placeholder — DT-01) |
| Mobile Client | React Native (future sprint) |
| Logging | `log/slog` JSON handler (stdlib) |

---

## Quick Start

### Prerequisites

| Tool | Version | Check |
|------|---------|-------|
| Go | 1.23+ | `go version` |
| Node.js | 20+ (for mobile) | `node --version` |
| Make | any | `make --version` |
| golangci-lint | latest | `golangci-lint --version` |
| OpenAI API key | — | `echo $OPENAI_API_KEY` |

### Setup

```bash
# Clone the repository
git clone https://github.com/Sergiotsk/TalkGo.git
cd TalkGo

# Install development tools (linters, formatters)
make setup

# Set your OpenAI API key (required for translation)
export OPENAI_API_KEY="sk-..."
```

### Running the Server

```bash
go run ./cmd/server -log-level debug
```

The server starts on `:8080` by default with JSON structured logging. Only one flag is available:

| Flag | Default | Values |
|------|---------|--------|
| `-log-level` | `info` | `debug`, `info`, `warn`, `error` |

The address is configured via `httpserver.DefaultConfig()` (`:8080`). To change it, modify the config in `cmd/server/main.go` or expose a new flag.

### Smoke Test

```bash
# 1. Create a room
curl -s -X POST http://localhost:8080/rooms \
  -H "Content-Type: application/json" \
  -d '{"source_lang":"es","target_lang":"en"}'

# Response: {"room_id":"<uuid>","short_code":"ABC123"}

# 2. Health check
curl -s http://localhost:8080/health
# Response: {"status":"ok"}

# 3. Look up room by short code
curl -s http://localhost:8080/rooms/code/ABC123
```

### WebSocket Connection

Connect two WebSocket clients to the same room to start a translation session:

```bash
# Client A (Spanish speaker)
websocat ws://localhost:8080/ws/<roomID>

# Send signaling messages as JSON:
# {"type":"join","user_id":"alice","room_id":"<roomID>","lang":"es"}
# Receive: {"type":"joined","sessionID":"<sessionID>","roomID":"<roomID>"}
# Send: {"type":"offer","sessionID":"<sessionID>","sdp":"..."}
```

The session starts when both peers have joined (room is full). The pipeline emits structured JSON logs for every event.

---

## Project Structure

```
TalkGo/
├── cmd/
│   ├── server/                 # Server entrypoint (main.go)
│   └── loadgen/                # WebSocket load generator tool
│       ├── main.go             # CLI flags, room creation
│       ├── session.go          # WS connect + signaling + RTT measurement
│       ├── audio.go            # Synthetic PCM tone generator
│       └── report.go           # JSON report with p50/p90 latency
│
├── internal/
│   ├── domain/                 # Pure domain model (no external imports)
│   │   ├── room/               # Room aggregate (Join, Leave, state)
│   │   └── session/            # Session lifecycle (Connecting → Active → Disconnected)
│   │
│   ├── ports/
│   │   ├── driven/             # Outbound port interfaces
│   │   │   ├── room_repository.go
│   │   │   ├── webrtc_peer.go
│   │   │   ├── translator.go
│   │   │   ├── audio_codec.go
│   │   │   ├── event_notifier.go
│   │   │   ├── audio_mixer.go
│   │   │   └── mocks/          # Generated mocks for testing
│   │   └── driving/            # Inbound port interfaces
│   │       ├── room_manager.go
│   │       └── signaling.go
│   │
│   ├── app/
│   │   └── roomsvc/            # Service layer (orchestration)
│   │       ├── service.go      # Room CRUD, session lifecycle, sweeper
│   │       ├── pipeline.go     # Bidirectional translation pipeline
│   │       ├── latency.go      # LatencyTracker + sync.Pool
│   │       ├── repository.go   # InMemoryRoomRepository
│   │       └── *_test.go       # Tests with log capture
│   │
│   └── adapters/               # Infrastructure implementations
│       ├── http/               # HTTP server (routes, handlers)
│       ├── signaling/          # WebSocket hub + client (gorilla/websocket)
│       ├── webrtc/             # Pion WebRTC adapter
│       ├── translator/         # OpenAI Realtime adapter
│       └── codec/              # Passthrough codec (placeholder)
│
├── scripts/
│   └── network-test/           # Network simulation toolkit
│       ├── simulate-4g.sh      # Linux: tc/netem network conditioning
│       ├── simulate-4g.ps1     # Windows: netsh bandwidth throttling
│       ├── run-test-session.sh # End-to-end automated test (Linux)
│       ├── run-test-session.ps1# End-to-end automated test (Windows)
│       ├── configs/            # Network profiles (YAML)
│       │   ├── 4g.yml
│       │   ├── wifi-home.yml
│       │   ├── wifi-cafe.yml
│       │   └── wan-lossy.yml
│       └── README.md           # Network testing documentation
│
├── mobile/                     # React Native client (future sprint)
├── docs/
│   ├── devel/                  # Developer documentation (this file)
│   ├── sprints/                # Sprint plans and retrospectives
│   ├── adr/                    # Architecture Decision Records
│   └── ...                     # PRD, specs, etc.
│
└── openspec/                   # Spec-Driven Design artifacts
    └── changes/
        └── sprint-4/           # Current sprint specs, design, tasks
```

---

## Configuration

### Command-line flags (cmd/server/main.go)

| Flag | Default | Description |
|------|---------|-------------|
| `-log-level` | `info` | Log level: `debug`, `info`, `warn`, `error` |

### ServiceConfig (internal/app/roomsvc/service.go)

| Field | Default | Description |
|-------|---------|-------------|
| `GracePeriod` | `30s` | Time a room stays alive after last disconnect |
| `RoomTTL` | `10m` | Max inactivity before room is swept |
| `SweepInterval` | `60s` | Frequency of expiration sweep |
| `MaxShortCodeRetries` | `5` | Collision retries for short code generation |

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `OPENAI_API_KEY` | Yes | API key for OpenAI Realtime translation |

---

## Key Concepts

### Pipeline

The translation pipeline processes audio in one direction (A to B or B to A). Each direction runs in its own goroutine, managed by a `pipelineHalf` struct.

The 5 instrumented stages of `runHalf()`:

| # | Stage | Code Location | What It Measures |
|---|-------|---------------|------------------|
| 1 | `capture` | `OnAudioTrack` callback | Time to receive a frame from the WebRTC track |
| 2 | `decode` | `s.codec.Decode(ctx, opusCh)` | Duration of the full Decode streaming call |
| 3 | `translate` | `s.translator.TranslateStream(ctx, bpCh, ...)` | Duration of the full TranslateStream call |
| 4 | `encode` | `s.codec.Encode(ctx, translatedCh)` | Duration of the full Encode streaming call |
| 5 | `send` | `s.peer.SendAudio(ctx, ...)` | Duration of the full SendAudio streaming call |

Each stage is wrapped with `tracker.StartStage()` / `tracker.EndStage()`. Because the pipeline stages use Go channels and run concurrently (each stage reads from the previous stage's output channel), stage timings are per-call, not per-frame. The `total_ms` in `chunk_latency` is the sum of all stage durations.

On error, the tracker emits `chunk_latency` with `status: "error"` and the stage timing up to the failure point. Error counts are incremented atomically via `errorChunks` (atomic.Int64).

### Goroutine Model

```
Room (pipeline.startPipeline)
  ├── goroutine: runHalf(A→B)
  │     ├── OnAudioTrack callback (per frame)
  │     ├── codec.Decode goroutine
  │     ├── buffer bridge goroutine (drainOldest)
  │     └── ... (stages via channels)
  └── goroutine: runHalf(B→A)
        └── (same structure)
  └── goroutine: wg.Wait() → emits pipeline_stop
```

Both halves run in parallel. The pipeline stops when either peer disconnects (context cancellation).

### LatencyTracker

`LatencyTracker` lives in `internal/app/roomsvc/latency.go` and provides per-chunk timing with minimal allocations:

- Uses `sync.Pool` to recycle `ChunkLatency` structs
- Pre-sizes `StageTimings` slice to capacity 5 (one per stage)
- Backing array reused across chunks via `[:0]`
- **< 3 allocations per chunk** after warmup (verified by `BenchmarkLatencyTracker`)
- Mutex-safe but uncontended (each pipeline half has its own tracker)

Key methods:

| Method | Behaviour |
|--------|-----------|
| `Reset()` | Increments chunkID, acquires ChunkLatency from pool, clears StageTimings |
| `StartStage(stage)` | Appends a StageTiming with `StartAt = time.Now()` |
| `EndStage(stage)` | Finds the last unclosed entry for the stage, sets EndAt, calculates Duration |
| `Emit(ctx, half, roomID, status)` | Calculates total, emits `chunk_latency` log, returns struct to pool |

### Logging

Logging uses `slog.NewJSONHandler(os.Stdout, ...)` with structured JSON format.

**Format conventions**:

| Convention | Rule | Example |
|------------|------|---------|
| Message key | snake_case identifier | `chunk_latency`, `session_start`, `server_starting` |
| Component | Always present | `component: "service"`, `component: "pipeline"` |
| Error field | Use `slog.Any("err", err)` | `slog.Any("err", err)` |
| Room context | Include `room_id` when available | — |
| Session context | Include `session_id` when available | — |

**Component values**:

| Component | Source |
|-----------|--------|
| `main` | `cmd/server/main.go` — startup/shutdown |
| `http` | `internal/adapters/http/server.go` |
| `hub` | `internal/adapters/signaling/hub.go`, `client.go` |
| `service` | `internal/app/roomsvc/service.go` |
| `pipeline` | `internal/app/roomsvc/pipeline.go`, `latency.go` |
| `loadgen` | `cmd/loadgen/` |

**Emission pattern**: Use `slog.LogAttrs()` for structured events with many fields (avoids heap allocation from key-value boxing). Use `slog.Info()` / `slog.Error()` for simple messages (fewer than ~4 fields).

### Session Lifecycle

```
Client connects via WS
  │
  ▼
JoinRoom(userID, roomID, lang)
  │
  ▼
Session created (State: Connecting)
  │
  ▼
Peer completes WebRTC handshake
  │
  ▼
Room becomes full (both peers joined)
  │
  ▼
startPipeline(sessA, sessB)
  ├── emits session_start (×2, one per peer)
  ├── emits pipeline_start
  ├── launches 2× runHalf goroutines (A→B, B→A)
  │
  ▼
Audio flows bidirectionally
  ├── chunk_latency emitted per stage
  └── session_error on pipeline failures
  │
  ▼ (one of:)
  ├── LeaveRoom() → session_end (event_type: "voluntary")
  ├── OnDisconnect → session_end (event_type: "disconnect")
  │     └── grace timer → session_end (event_type: "timeout") + DeleteRoom
  └── Room expires → sweepExpiredRooms → DeleteRoom
```

---

## Event Reference

### session_event

| event | When | Key Fields |
|-------|------|------------|
| `session_start` | After JoinRoom, both peers ready | `session_id`, `user_id`, `lang`, `room_id`, `component: "service"` |
| `session_end` | LeaveRoom / OnDisconnect / grace timeout | `session_id`, `room_id`, `user_id`, `duration_sec`, `event_type` (voluntary/disconnect/timeout), `component: "service"` |
| `session_error` | Pipeline stage failure | `session_id`, `error`, `error_count`, `stage`, `component: "pipeline"` |
| `pipeline_start` | Before launching runHalf goroutines | `room_id`, `sessA`, `sessB`, `langA`, `langB`, `component: "pipeline"` |
| `pipeline_stop` | Both runHalf goroutines complete | `room_id`, `total_chunks_AtoB`, `total_chunks_BtoA`, `component: "pipeline"` |

### chunk_latency

| Field | Type | Description |
|-------|------|-------------|
| `component` | string | Always `"pipeline"` |
| `room_id` | string | Room UUID |
| `half` | string | `"AtoB"` or `"BtoA"` |
| `chunk_id` | string | Auto-incrementing counter (1-based, per half) |
| `total_ms` | int64 | Sum of all stage durations in milliseconds |
| `stages` | object | `{ "capture_ms": N, "decode_ms": N, "translate_ms": N, "encode_ms": N, "send_ms": N }` |
| `status` | string | `"ok"` or `"error"` |

### Error Events

| Message | Component | Fields |
|---------|-----------|--------|
| `close_session_error` | service | `session_id`, `err` |
| `grace_timer_delete_error` | service | `room_id`, `err` |
| `sweep_list_error` | service | `err` |
| `sweep_delete_error` | service | `room_id`, `err` |
| `on_disconnect_error` | hub | `session_id`, `err` |
| `ws_upgrade_failed` | hub | `err` |
| `signal_response_marshal_error` | hub | `err` |
| `notify_session_marshal_error` | hub | `err` |
| `ws_write_error` | hub | `err` |
| `ws_read_error` | hub | `err` |
| `create_room_error` | http | `err` |
| `delete_room_error` | http | `err` |
| `find_by_code_error` | http | `err` |
| `ws_handler_error` | http | `err` |
| `http_listening_error` | http | `err` |

### Lifecycle Events

| Message | Component | Fields |
|---------|-----------|--------|
| `server_starting` | main | `addr` |
| `shutdown_starting` | main | — |
| `service_creation_failed` | main | `err` |
| `server_error` | http | `err` |
| `http_listening` | http | `addr` |
| `http_shutdown` | http | — |
| `http_stopped` | http | — |

---

## Network Testing

### Load Generator

The loadgen tool in `cmd/loadgen/` simulates a WebSocket peer for network testing. It connects to a TalkGo server, follows the signaling protocol, measures RTT, and produces a JSON report.

```bash
# Quick 30-second test
go run ./cmd/loadgen -server localhost:8080 -duration 30s

# All flags
go run ./cmd/loadgen -server localhost:8080 -room <roomID> -lang en -duration 60s -profile 4g -output report.json
```

| Flag | Default | Description |
|------|---------|-------------|
| `-server` | `localhost:8080` | TalkGo server address |
| `-room` | auto-create | Room ID (auto-created via POST /rooms if empty) |
| `-lang` | `es` | Peer language code |
| `-duration` | `30s` | Test duration |
| `-profile` | `wifi-home` | Network profile label for the report |
| `-output` | stdout | Report output file path |

### Network Simulation (Linux)

```bash
# Apply 4G profile
sudo ./scripts/network-test/simulate-4g.sh -Profile 4g

# Run a test session
./scripts/network-test/run-test-session.sh -Profile 4g -Duration 30s

# Reset network rules
sudo ./scripts/network-test/simulate-4g.sh -Reset
```

### Network Simulation (Windows)

```powershell
# Requires PowerShell as Administrator
.\scripts\network-test\simulate-4g.ps1 -Profile 4g

# Run a test session
.\scripts\network-test\run-test-session.ps1 -Profile wifi-home -Duration 30s

# Reset
.\scripts\network-test\simulate-4g.ps1 -Reset
```

> **Limitation**: Windows `netsh` cannot simulate packet loss or latency. For full network simulation (RTT, loss, jitter), use WSL2 or native Linux.

### Network Profiles

| Profile | Bandwidth | RTT | Loss | Jitter | Use Case |
|---------|-----------|-----|------|--------|----------|
| `4g` | 10 Mbps | 100 ms | 5% | 10 ms | Mobile network |
| `wifi-cafe` | 5 Mbps | 150 ms | 8% | 20 ms | Congested public WiFi |
| `wifi-home` | 50 Mbps | 20 ms | 1% | 2 ms | Residential WiFi |
| `wan-lossy` | 2 Mbps | 300 ms | 15% | 30 ms | Severe WAN loss |

### Parsing Logs with jq

```bash
# Stream only chunk_latency events
go run ./cmd/server | jq 'select(.msg == "chunk_latency")'

# Extract latency distribution
go run ./cmd/server | jq 'select(.msg == "chunk_latency") | .total_ms' | sort -n | \
  awk '{a[NR]=$1} END{print "p50:", a[int(NR*0.5)], "p90:", a[int(NR*0.9)]}'

# Watch session events
go run ./cmd/server | jq 'select(.msg == "session_event")'
```

---

## Common Tasks

### Adding a New Log Event

1. Choose a snake_case identifier (e.g., `peer_connected`)
2. Add the `component` field with the appropriate value
3. Include relevant context fields (`room_id`, `session_id`, etc.)
4. Use `slog.LogAttrs()` for events with 4+ fields
5. Use `slog.Any("err", err)` for error values

```go
slog.LogAttrs(ctx, slog.LevelInfo, "peer_connected",
    slog.String("component", "hub"),
    slog.String("room_id", roomID),
    slog.String("session_id", sessionID),
)
```

### Adding a Pipeline Stage

1. Add a stage constant in `internal/app/roomsvc/latency.go`
2. Add the corresponding `_ms` key in `stageMsKey()` function
3. In `runHalf()` in `pipeline.go`, add `tracker.StartStage(YourStage)` before the operation and `tracker.EndStage(YourStage)` after
4. Handle errors: increment `half.errorChunks.Add(1)`, call `tracker.Emit(ctx, half.dir, roomID, "error")`, and return
5. Update the `Attr` arrays in `Emit()` if the stage count exceeds 5

### Writing Tests

Tests use **log capture** to verify structured log output:

```go
func TestSomething(t *testing.T) {
    var buf bytes.Buffer
    logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
    old := slog.Default()
    slog.SetDefault(logger)
    defer slog.SetDefault(old)

    // ... your test that triggers log emission ...

    // Assert on captured JSON
    var logEntry map[string]any
    json.Unmarshal(buf.Bytes(), &logEntry)
    assert.Equal(t, "chunk_latency", logEntry["msg"])
}
```

**Test package strategy**: Use external test packages (`roomsvc_test`) for integration-style tests that verify behaviour through public API. Use internal test packages (`roomsvc`) for unit tests that need access to unexported symbols.

**Race detection**: All tests run with `-race` in CI. Use `lockedBuffer` or dedicated log capture per test to avoid data races on shared log output.

### Platform-Specific Test Concerns

| Concern | Solution |
|---------|----------|
| Data races from shared log output | Use separate `bytes.Buffer` per test |
| Grace timer waits in tests | Set `GracePeriod: 1 * time.Millisecond` in test config |
| Concurrent pipeline access | Each half has its own LatencyTracker (no shared state) |
| Network-dependent tests | Mock all driven ports (`WebRTCPeer`, `Translator`, `AudioCodec`) |

---

## Troubleshooting

### Build Errors

| Error | Likely Cause | Fix |
|-------|-------------|-----|
| `undefined: slog` | Go version < 1.21 | Upgrade to Go 1.23 |
| `package not found` | Missing dependency | `go mod tidy` |
| `cgo: C compiler exec` | CGO_ENABLED on Windows | `set CGO_ENABLED=0` |
| `golangci-lint not found` | Linter not installed | `make setup` |

### Runtime Issues

| Issue | Likely Cause | Fix |
|-------|-------------|-----|
| Server won't start | Port 8080 in use | Kill process: `lsof -ti:8080 \| xargs kill` (Linux) or `netstat -ano \| findstr :8080` (Windows) |
| Translation fails | Missing `OPENAI_API_KEY` | Set environment variable with a valid key |
| WebSocket won't connect | Room doesn't exist | Create room via `POST /rooms` first |
| Pipeline doesn't start | Only one peer joined | Both peers must join before pipeline starts |
| Pion ICE failures | No STUN connectivity | Check network, try `DefaultConfig()` |
| High latency | Network conditions | Run with `-log-level debug` and inspect `chunk_latency` logs |

### Race Conditions in Tests

If you see `WARNING: DATA RACE` in test output:

1. Check for shared `slog` default logger — use per-test log capture
2. Check for shared `bytes.Buffer` across goroutines — use separate buffers
3. Verify mutex locking in `Service` methods (all session/pipeline access goes through `s.mu`)
4. Run with `go test -race -count=1` for reproducible results

### Windows-Specific

- Packet loss and latency simulation requires WSL2 — native Windows `netsh` only supports bandwidth throttling
- For bandwidth limiting: `netsh int tcp set global autotuninglevel=disabled`
- All scripts require PowerShell as Administrator
- Use `choco install jq` to get `jq` on Windows

### Grace Timer in Tests

In production, `GracePeriod` is 30 seconds. In tests, set it to 1 millisecond to keep tests fast:

```go
cfg := roomsvc.ServiceConfig{
    GracePeriod:   1 * time.Millisecond,
    // ... other fields ...
}
```

---

## Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Compile all packages (`go build ./...`) |
| `make test` | Run tests with race detector and coverage (`go test -race -cover ./...`) |
| `make test-verbose` | Run tests with verbose output |
| `make lint` | Run golangci-lint |
| `make fmt` | Format all Go files (gofmt + goimports) |
| `make check` | Format, lint, and test (CI gate) |
| `make setup` | Install development tools |
| `make cover` | Generate HTML coverage report |
| `make clean` | Clear test cache |
| `make help` | Show all targets |
