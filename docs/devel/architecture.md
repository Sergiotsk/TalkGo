# TalkGo Architecture

## Hexagonal Architecture Overview

TalkGo follows strict **Hexagonal Architecture (Ports & Adapters)**, also known as the Onion Architecture or Clean Architecture. The core principle: domain logic has zero knowledge of the outside world. All communication crosses boundary interfaces defined as ports.

```
                    ┌──────────────────────────────────────────────────┐
                    │                 DRIVING SIDE                     │
                    │  (what the world asks of our application)        │
                    │                                                  │
                    │  ┌──────────┐    ┌──────────┐    ┌──────────┐   │
                    │  │   HTTP   │    │WebSocket │    │   CLI    │   │
                    │  │ Adapter  │    │  Hub     │    │ (loadgen)│   │
                    │  └────┬─────┘    └────┬─────┘    └────┬─────┘   │
                    │       │               │               │         │
                    │       ▼               ▼               │         │
                    │  ┌─────────────────────────────────────┐        │
                    │  │        DRIVING PORTS (inbound)      │        │
                    │  │  ┌──────────────┐ ┌──────────────┐  │        │
                    │  │  │ RoomManager  │ │SignalingHandler│ │        │
                    │  │  └──────┬───────┘ └──────┬─────────┘ │        │
                    │  └─────────┼────────────────────────────┘        │
                    └────────────┼─────────────────────────────────────┘
                                 │
                    ┌────────────┼─────────────────────────────────────┐
                    │            ▼                                      │
                    │  ┌─────────────────────────────────────────────┐ │
                    │  │         APPLICATION LAYER                   │ │
                    │  │    internal/app/roomsvc/                    │ │
                    │  │                                             │ │
                    │  │  ┌──────────┐ ┌──────────┐ ┌──────────┐   │ │
                    │  │  │ Service  │ │ Pipeline │ │ Latency  │   │ │
                    │  │  │ (CRUD)   │ │(runHalf) │ │ Tracker  │   │ │
                    │  │  └────┬─────┘ └────┬─────┘ └──────────┘   │ │
                    │  │       │             │                       │ │
                    │  │  ┌────▼─────────────▼─────┐                │ │
                    │  │  │   Repository (memory)  │                │ │
                    │  │  └────────────────────────┘                │ │
                    │  └─────────────────────────────────────────────┘ │
                    └────────────┬─────────────────────────────────────┘
                                 │
                    ┌────────────┼─────────────────────────────────────┐
                    │            ▼                                      │
                    │  ┌─────────────────────────────────────────────┐ │
                    │  │        DRIVEN PORTS (outbound)              │ │
                    │  │  ┌──────────┐┌──────────┐┌──────┐┌──────┐ │ │
                    │  │  │WebRTCPeer││Translator││Codec ││Repo │ │ │
                    │  │  └────┬─────┘└────┬─────┘└──┬───┘└──┬───┘ │ │
                    │  └───────┼───────────┼─────────┼───────┼──────┘ │
                    └──────────┼───────────┼─────────┼───────┼────────┘
                               │           │         │       │
                    ┌──────────▼───────────▼─────────▼───────▼────────┐
                    │              DRIVEN ADAPTERS                    │
                    │  ┌──────────┐ ┌──────────┐ ┌──────┐ ┌────────┐ │
                    │  │  Pion    │ │ OpenAI   │ │Pass- │ │In-Mem  │ │
                    │  │ WebRTC   │ │ Realtime │ │through│ │ Store   │ │
                    │  └──────────┘ └──────────┘ └──────┘ └────────┘ │
                    │                                                 │
                    └─────────────────────────────────────────────────┘
```

### Rule of thumb

- `internal/domain/` imports NOTHING from outside the domain package
- `internal/ports/` defines interfaces, imports only domain types
- `internal/app/roomsvc/` imports domain + ports (application logic)
- `internal/adapters/` imports ports (infrastructure implementations)
- `cmd/` imports adapters (wiring/DI)

This is enforced by `golangci-lint` with `depguard` — any package that violates the layer rule fails the build.

---

## Package Dependency Graph

```
cmd/server
  └── internal/adapters/http
        └── internal/adapters/signaling
              └── internal/domains/room, session  (via ports)
        └── internal/ports/driving
              └── internal/domain/room, session
  └── internal/app/roomsvc
        ├── internal/domain/room
        ├── internal/domain/session
        ├── internal/ports/driving
        ├── internal/ports/driven
        └── internal/adapters/signaling (as EventNotifier)
  └── internal/adapters/webrtc
        └── internal/ports/driven
  └── internal/adapters/translator
        └── internal/ports/driven
  └── internal/adapters/codec
        └── internal/ports/driven

cmd/loadgen
  └── (external only: gorilla/websocket, Go stdlib)
  └── NO imports of internal/ packages (NFR-08)
```

### Dependency direction

```
domain  ←  ports  ←  app  ←  adapters  ←  cmd
```

The domain layer sits at the centre with zero external dependencies. Each layer outward depends on the layer inward through interfaces.

---

## Data Flow: Audio Translation

### High-Level Flow

```
Speaker A (Spanish)                  Speaker B (English)
      │                                     ▲
      │ Opus frames via WebRTC              │ Opus frames via WebRTC
      ▼                                     │
┌─────────────┐                   ┌─────────────────┐
│  WebSocket  │                   │   WebSocket      │
│  + WebRTC   │                   │   + WebRTC       │
└──────┬──────┘                   └────────┬─────────┘
       │                                   │
       │ signaling/hub routes to roomsvc   │
       ▼                                   │
┌──────────────────────────────────────────┐
│            Service Layer                 │
│  ┌────────────────────────────────────┐  │
│  │         Pipeline                   │  │
│  │  ┌──────────┐   ┌──────────┐     │  │
│  │  │ runHalf  │   │ runHalf  │     │  │
│  │  │ (A→B)    │   │ (B→A)    │     │  │
│  │  │ es→en    │   │ en→es    │     │  │
│  │  └────┬─────┘   └────┬─────┘     │  │
│  └───────┼──────────────┼───────────┘  │
└──────────┼──────────────┼──────────────┘
           │              │
           ▼              ▼
    ┌──────────┐   ┌──────────┐
    │ OpenAI   │   │ OpenAI   │
    │ Realtime │   │ Realtime │
    │ es→en    │   │ en→es    │
    └──────────┘   └──────────┘
```

### Per-Chunk Data Flow (one half)

```
OnAudioTrack callback
  │
  ▼
[Stage 1: Capture] — tracker.StartStage/EndStage (instant, frame in memory)
  │
  ▼
opusCh (chan []byte, buffer 8)
  │
  ▼
[Stage 2: Decode] — s.codec.Decode(ctx, opusCh) → pcmCh
  │                          tracker.StartStage/EndStage wraps the call
  ▼
pcmCh (chan []byte, unbuffered)
  │
  ▼
drainOldest buffer bridge → bpCh (chan []byte, buffer 1)
  │
  ▼
[Stage 3: Translate] — s.translator.TranslateStream(ctx, bpCh, source, target)
  │                          tracker.StartStage/EndStage wraps the call
  ▼
translatedCh (chan []byte)
  │
  ▼
[Stage 4: Encode] — s.codec.Encode(ctx, translatedCh) → opusOutCh
  │                          tracker.StartStage/EndStage wraps the call
  ▼
opusOutCh (chan []byte)
  │
  ▼
[Stage 5: Send] — s.peer.SendAudio(ctx, targetSessID, opusOutCh)
                     tracker.StartStage/EndStage wraps the call
  │
  ▼
Peer B receives Opus audio
```

### Notes on the Flow

- Each stage returns a Go channel. Stages run concurrently — the decoder can decode frame N while the translator processes frame N-1.
- Between Decode and Translate, a `drainOldest` buffer bridge ensures the translator always receives the most recent audio frame (discards stale frames if backlogged).
- The `chunk_latency` log is emitted once per pipeline half, when `SendAudio` completes. `total_ms` is the sum of per-stage durations, not wall-clock time (stages overlap).
- On error at any stage, the tracker emits `chunk_latency` with `status: "error"` and the pipeline half returns immediately.

---

## Pipeline Internals

### pipelineHalf struct

```go
type pipelineHalf struct {
    sourceSessID string
    targetSessID string
    sourceLang   string
    targetLang   string
    dir          string            // "AtoB" or "BtoA"
    tracker      *LatencyTracker   // per-half latency tracker
    totalChunks  atomic.Int64      // chunks processed successfully
    errorChunks  atomic.Int64      // chunks with errors
}
```

### startPipeline() — Lifecycle Orchestrator

1. Creates a cancellable context for the pipeline
2. Registers the pipeline in `Service.pipelines[roomID]`
3. Creates two `pipelineHalf` instances (AtoB, BtoA), each with its own `LatencyTracker`
4. Emits `session_start` for each peer
5. Emits `pipeline_start` with session IDs and language codes
6. Launches two goroutines: `runHalf(ctx, p, &halfAtoB)` and `runHalf(ctx, p, &halfBtoA)`
7. Spawns a third goroutine that waits for both halves to finish (`p.wg.Wait()`) and emits `pipeline_stop` with chunk counters

### runHalf() — The 5-Stage Pipeline

`runHalf` is the core processing function. It:

1. Sets up `OnAudioTrack` callback that:
   - Receives audio frames from the WebRTC track
   - Calls `tracker.Reset()` and `tracker.StartStage(StageCapture)` per frame
   - Sends frames to `opusCh`
2. Calls `s.codec.Decode(ctx, opusCh)` wrapped in Decode timing
3. Bridges the decoded PCM channel through `drainOldest`
4. Calls `s.translator.TranslateStream(ctx, bpCh, ...)` wrapped in Translate timing
5. Calls `s.codec.Encode(ctx, translatedCh)` wrapped in Encode timing
6. Calls `s.peer.SendAudio(ctx, ...)` wrapped in Send timing
7. On success: increments `totalChunks` with frame count, emits `chunk_latency` with `status: "ok"`
8. On error at any stage: increments `errorChunks`, emits `chunk_latency` with `status: "error"`, returns

### Goroutine Model

```
Room
  │
  ├── goroutine: startPipeline (caller)
  │
  ├── goroutine: runHalf(A→B)
  │     │
  │     ├── OnAudioTrack callback goroutine (per frame)
  │     ├── buffer bridge: pcmCh → bpCh (drainOldest loop)
  │     └── SendAudio blocks until channel closes
  │
  ├── goroutine: runHalf(B→A)
  │     └── (same structure)
  │
  └── goroutine: wg.Wait → pipeline_stop
```

Each room gets **3 additional goroutines** beyond the two `runHalf` calls: two buffer bridge goroutines (one per half) and one pipeline_stop waiter. When the pipeline is cancelled (context cancellation), all goroutines exit via `ctx.Done()` checks.

---

## Latency Instrumentation

### LatencyTracker Design

`LatencyTracker` provides per-chunk timing with minimal allocations, designed for high-frequency audio frame processing (theoretically up to 50 frames/second per half).

```
                    sync.Pool
                        │
              ┌─────────▼──────────┐
              │  ChunkLatency #1   │◄── in use by tracker
              │  ChunkLatency #2   │── returned after Emit()
              │  ChunkLatency #3   │
              │  ...               │
              └────────────────────┘
```

### Allocation Budget

After warmup (sync.Pool populated), each chunk allocates:

| Allocation | Source | Size |
|-----------|--------|------|
| `ChunkID` string | `strconv.FormatInt` in `Reset()` | ~3-8 bytes |
| (internal) | `slog.LogAttrs` attribute arrays | stack-allocated (fixed-size arrays) |
| (reused) | `StageTimings` backing array | recycled via `[:0]` |

**Target: < 3 allocations per chunk**, verified by `BenchmarkLatencyTracker_Allocations`.

### Where Stage Timing is Captured

| Stage | File | Start | End |
|-------|------|-------|-----|
| `capture` | `pipeline.go:149` | Frame received from `trackCh` | Immediately after (instant) |
| `decode` | `pipeline.go:168` | Before `s.codec.Decode(ctx, opusCh)` | After Decode returns a channel |
| `translate` | `pipeline.go:198` | Before `s.translator.TranslateStream(...)` | After TranslateStream returns a channel |
| `encode` | `pipeline.go:211` | Before `s.codec.Encode(ctx, translatedCh)` | After Encode returns a channel |
| `send` | `pipeline.go:224` | Before `s.peer.SendAudio(...)` | After SendAudio returns |

### Emit Format

The `Emit()` method constructs the JSON log entry using fixed-size attribute arrays (no heap allocation for slices):

```go
slog.LogAttrs(ctx, slog.LevelInfo, "chunk_latency",
    slog.String("component", "pipeline"),
    slog.String("room_id", roomID),
    slog.String("half", half),
    slog.String("chunk_id", cl.ChunkID),
    slog.String("status", status),        // "ok" or "error"
    slog.Int64("total_ms", total.Milliseconds()),
    slog.Group("stages",
        slog.Int64("capture_ms", captureMs),
        slog.Int64("decode_ms", decodeMs),
        slog.Int64("translate_ms", translateMs),
        slog.Int64("encode_ms", encodeMs),
        slog.Int64("send_ms", sendMs),
    ),
)
```

---

## Session Events

### Lifecycle Diagram

```
                    JoinRoom(userA, roomX, "es")
                           │
                           ▼
                    ┌──────────────┐
                    │  Session A   │
                    │  Connecting  │
                    └──────┬───────┘
                           │
                    JoinRoom(userB, roomX, "en")
                           │
                           ▼
                    Room becomes FULL
                           │
                           ▼
              startPipeline(sessA, sessB)
              ├── session_start (sessA)
              ├── session_start (sessB)
              └── pipeline_start
                           │
                           ▼
              ┌────────────────────────┐
              │  runHalf(A→B)          │
              │  runHalf(B→A)          │
              │  chunk_latency × N     │
              │  (session_error × M)   │
              └───────────┬────────────┘
                          │
              ┌───────────┴───────────┐
              │                       │
         LeaveRoom()            OnDisconnect()
              │                       │
              ▼                       ▼
    session_end(voluntary)   session_end(disconnect)
              │                       │
              │                 Grace Timer starts
              │                       │
              │              (reconnect?)   (timeout)
              │                       │
              │                       ▼
              │               session_end(timeout)
              │               DeleteRoom()
              │                       │
              └─────── pipeline stop ──┘
              pipeline_stop emitted
```

### Event Ownership

| Event | Emitter | File |
|-------|---------|------|
| `session_start` | `startPipeline()` | `pipeline.go:88-103` |
| `session_end` (voluntary) | `LeaveRoom()` | `service.go:311-319` |
| `session_end` (disconnect) | `OnDisconnect()` | `service.go:377-385` |
| `session_end` (timeout) | Grace timer callback | `service.go:393-401` |
| `session_error` | `logSessionError()` helper | `pipeline.go:240-255` |
| `pipeline_start` | `startPipeline()` | `pipeline.go:105-113` |
| `pipeline_stop` | `wg.Wait()` goroutine | `pipeline.go:124-130` |
| `chunk_latency` | `LatencyTracker.Emit()` | `latency.go:139-181` |

### session_end Fields

The actual field used for the end reason is `event_type` (not `reason` as in earlier designs):

```go
slog.LogAttrs(ctx, slog.LevelInfo, "session_event",
    slog.String("event", "session_end"),
    slog.String("session_id", sessionID),
    slog.String("room_id", roomID),
    slog.String("user_id", sess.UserID),
    slog.Int64("duration_sec", int64(time.Since(sess.JoinedAt).Seconds())),
    slog.String("event_type", "voluntary"),   // or "disconnect" / "timeout"
    slog.String("component", "service"),
)
```

---

## Deployment

### Build

TalkGo compiles to a **single static binary**:

```bash
go build -o talkgo-server ./cmd/server
go build -o talkgo-loadgen ./cmd/loadgen
```

No build tags, no CGO dependency (the Opus codec is a passthrough placeholder — DT-01). The binary is fully self-contained.

### Deployment Architecture

```
                 Internet
                     │
              (WebSocket :8080)
                     │
              ┌──────┴──────┐
              │  TalkGo     │
              │  Server     │
              │  (single    │
              │   binary)   │
              └──────┬──────┘
                     │
          ┌──────────┴──────────┐
          │                     │
    OpenAI Realtime         STUN Server
    API (cloud)             (google: stun.l.google.com:19302)
```

### Current Limitations

- **WebSocket-only transport**: No WebRTC data channels yet. Audio flows through Pion WebRTC tracks, but signaling is WebSocket-based.
- **No horizontal scaling**: Rooms are in-memory. A future sprint will add a Redis-backed repository for multi-instance deployment.
- **Passthrough codec**: The current `PassthroughCodec` does no real Opus encoding/decoding. Real codec integration (libopus) is tracked as DT-01.
- **OpenAI dependency**: Translation requires OpenAI Realtime API. No offline/fallback translation mode.
- **No authentication**: The server has no auth layer. Any client can create/join rooms.

### Graceful Shutdown

```
SIGINT/SIGTERM
  │
  ▼
cmd/server/main.go: signal handler calls cancel()
  │
  ├── HTTP server: Shutdown() with 15s timeout
  ├── Hub: RunCtx receives ctx.Done() → closes all WS connections
  ├── Pipeline: all runHalf goroutines exit via ctx.Done()
  └── Sweeper: StartExpirationSweep goroutine exits via ctx.Done()
  │
  ▼
Program exits with code 0 (or 1 on error)
```

The shutdown sequence ensures all WebSocket connections are closed gracefully and all pipeline goroutines exit before the process terminates. The `cancel()` function is called via `defer` in `run()`, which executes even if `return 1` is used (fixed in Sprint 4 — previously `os.Exit(1)` skipped defers).
