# Sprint 4 Technical Design: Polish y Alpha — Instrumentación, Logging, Network Testing

**Change**: sprint-4
**Status**: design
**Date**: 2026-06-11
**Inputs**: proposal (openspec/changes/sprint-4/proposal.md), spec (openspec/changes/sprint-4/spec.md), codebase analysis

---

## Section 1: Workstream A — Instrumentación + Logging

### 1.1 LatencyTracker Package (`internal/app/roomsvc/latency.go`)

**Design decision**: New file `latency.go` in `internal/app/roomsvc/` (not a new package). The types and tracker live alongside the pipeline code that uses them. A separate package would add import complexity for zero benefit at this scale.

#### `ChunkLatency` struct — exact definition

```go
// Stage names exposed as package constants for use by pipeline instrumentation.
const (
    StageCapture   = "capture"
    StageDecode    = "decode"
    StageTranslate = "translate"
    StageEncode    = "encode"
    StageSend      = "send"
)

// ChunkLatency holds per-chunk timing data. Stored as value (not pointer) in
// the sync.Pool to avoid heap escapes. The pool returns *ChunkLatency for
// convenience, but the underlying struct is a value type.
type ChunkLatency struct {
    ChunkID      string
    StageTimings []StageTiming
    Total        time.Duration
}

type StageTiming struct {
    Stage    string
    StartAt  time.Time
    EndAt    time.Time
    Duration time.Duration
}
```

**Design decision**: `ChunkLatency` is a **value struct**, not a pointer. The `sync.Pool` returns `*ChunkLatency` for call-site convenience, but the pool reuses the same backing array for `StageTimings`. This avoids allocation churn while keeping the API ergonomic.

#### `LatencyTracker` — exact struct and methods

```go
// LatencyTracker records per-chunk timing for one pipeline half.
// Each pipeline half gets its own tracker — no shared state.
type LatencyTracker struct {
    pool    *sync.Pool      // reuses ChunkLatency + StageTimings backing slice
    current *ChunkLatency   // in-flight chunk data
    chunkID int64           // auto-incrementing per half
    mu      sync.Mutex      // guards current and chunkID
}

// NewLatencyTracker creates a tracker with a sync.Pool that pre-allocates
// a ChunkLatency value and a 5-capacity StageTiming slice (matching the
// 5 pipeline stages). This minimizes re-slicing growth.
func NewLatencyTracker() *LatencyTracker { ... }

// StartStage records the start time for the given stage.
// Panics if called twice for the same stage without an EndStage in between.
// Safe for concurrent use — each pipeline half calls serially.
func (lt *LatencyTracker) StartStage(stage string) { ... }

// EndStage records the end time for the given stage.
// Calculates Duration = EndAt.Sub(StartAt).
// If called without a corresponding StartStage, it records zero start time.
func (lt *LatencyTracker) EndStage(stage string) { ... }

// Emit logs a structured chunk_latency event and resets the tracker for
// the next chunk. The ChunkLatency is returned to the pool after emission.
// Fields:
//   msg: "chunk_latency"
//   component: "pipeline"
//   room_id
//   half ("AtoB" or "BtoA")
//   chunk_id (int64, 1-based)
//   stages: { capture_ms, decode_ms, translate_ms, encode_ms, send_ms }
//   total_ms
//   status ("ok" or "error")
func (lt *LatencyTracker) Emit(ctx context.Context, half string, roomID string, status string) { ... }

// Reset prepares the tracker for a new chunk. Called at the top of each
// chunk iteration in runHalf. Increments chunkID and clears StageTimings.
func (lt *LatencyTracker) Reset() { ... }
```

**`sync.Pool` details**:

```go
pool := &sync.Pool{
    New: func() any {
        return &ChunkLatency{
            StageTimings: make([]StageTiming, 0, 5), // pre-size for 5 stages
        }
    },
}
```

**Design decision — method signatures**:
- `StartStage`/`EndStage` take a string stage name, NOT a typed constant. The five stage constants are package-level strings (`StageCapture`, `StageDecode`, etc.) that callers MUST use. String comparison is fast enough for this use case and avoids a circular dependency with a separate types package.
- `Emit` takes `ctx context.Context` (for structured logging fields) plus `half`, `roomID`, and `status`. These are caller-provided because the tracker doesn't own pipeline context.

**Design decision — thread safety**: `LatencyTracker` uses a `sync.Mutex` because each pipeline half goroutine calls sequentially (one chunk at a time), but `Emit` and `Reset` must be safe if called from defer/cleanup paths. The mutex is uncontended in practice — it's a safety guarantee.

#### Allocation efficiency

```go
func (lt *LatencyTracker) Reset() {
    lt.mu.Lock()
    defer lt.mu.Unlock()
    lt.chunkID++
    cl := lt.pool.Get().(*ChunkLatency)
    cl.ChunkID = fmt.Sprintf("%d", lt.chunkID)
    cl.StageTimings = cl.StageTimings[:0] // reset slice length, keep backing array
    cl.Total = 0
    lt.current = cl
}
```

The `StageTimings` backing array is reused across chunks. The `ChunkLatency` struct itself is recycled via `sync.Pool`. The `ChunkID` string is the only allocation per chunk (a small `strconv.FormatInt` or `fmt.Sprintf`).

Benchmark target: `< 3 allocations per chunk` (one for ChunkID string, two are internal to the pool). `testing.AllocsPerRun` in `BenchmarkLatencyTracker` must confirm this.

---

### 1.2 Pipeline Modifications (`internal/app/roomsvc/pipeline.go`)

#### Decision: Inline instrumentation (NOT a wrapper)

**Approach**: Instrument `runHalf()` directly with `StartStage()`/`EndStage()` calls. A wrapper function would need to duplicate the entire `runHalf` signature and lifecycle, adding complexity. Inline instrumentation is simpler to read, maintain, and debug.

#### Modified `pipelineHalf` struct — add tracker and counters

```go
type pipelineHalf struct {
    sourceSessID string
    targetSessID string
    sourceLang   string
    targetLang   string
    dir          string      // "AtoB" or "BtoA"

    tracker *LatencyTracker  // NEW: per-half latency tracker

    totalChunks  atomic.Int64 // NEW: chunks processed successfully
    errorChunks  atomic.Int64 // NEW: chunks with errors
}
```

**Design decision**: `totalChunks` and `errorChunks` are `atomic.Int64` (not `sync.Mutex`-guarded ints) because they are read by `pipeline_stop` from a potentially different goroutine. Atomic ops are lock-free and trivially safe. `atomic.Int64` requires Go 1.19+ (we have Go 1.23).

#### Modified `startPipeline` — wire tracker and emit events

```go
func (s *Service) startPipeline(r *room.Room, sessA, sessB *session.Session) {
    ctx, cancel := context.WithCancel(context.Background())
    p := &pipeline{
        ctx:    ctx,
        cancel: cancel,
        roomID: r.ID,
        sessA:  sessA,
        sessB:  sessB,
    }

    s.mu.Lock()
    s.pipelines[r.ID] = p
    s.mu.Unlock()

    halfAtoB := pipelineHalf{
        sourceSessID: sessA.ID,
        targetSessID: sessB.ID,
        sourceLang:   sessA.Lang,
        targetLang:   sessB.Lang,
        dir:          "AtoB",
        tracker:      NewLatencyTracker(),  // NEW
    }
    halfBtoA := pipelineHalf{
        sourceSessID: sessB.ID,
        targetSessID: sessA.ID,
        sourceLang:   sessB.Lang,
        targetLang:   sessA.Lang,
        dir:          "BtoA",
        tracker:      NewLatencyTracker(),  // NEW
    }

    // START: session events
    slog.LogAttrs(ctx, slog.LevelInfo, "session_event",
        slog.String("event", "session_start"),
        slog.String("session_id", sessA.ID),
        slog.String("user_id", sessA.UserID),
        slog.String("lang", sessA.Lang),
        slog.String("room_id", r.ID),
        slog.String("component", "service"),
    )
    slog.LogAttrs(ctx, slog.LevelInfo, "session_event",
        slog.String("event", "session_start"),
        slog.String("session_id", sessB.ID),
        slog.String("user_id", sessB.UserID),
        slog.String("lang", sessB.Lang),
        slog.String("room_id", r.ID),
        slog.String("component", "service"),
    )
    slog.LogAttrs(ctx, slog.LevelInfo, "session_event",
        slog.String("event", "pipeline_start"),
        slog.String("room_id", r.ID),
        slog.String("sessA", sessA.ID),
        slog.String("sessB", sessB.ID),
        slog.String("langA", sessA.Lang),
        slog.String("langB", sessB.Lang),
        slog.String("component", "pipeline"),
    )
    // END: session events

    p.wg.Add(2)
    go s.runHalf(ctx, p, halfAtoB)
    go s.runHalf(ctx, p, halfBtoA)

    // Wait for both halves to complete, then emit pipeline_stop.
    go func() {
        p.wg.Wait()
        slog.LogAttrs(ctx, slog.LevelInfo, "session_event",
            slog.String("event", "pipeline_stop"),
            slog.String("room_id", r.ID),
            slog.Int64("total_chunks_AtoB", halfAtoB.totalChunks.Load()),
            slog.Int64("total_chunks_BtoA", halfBtoA.totalChunks.Load()),
            slog.String("component", "pipeline"),
        )
    }()
}
```

**Design decision — who emits session events**: The **pipeline** (via `startPipeline`) emits `session_start` and `pipeline_start` because it is the orchestrator that knows when both peers are joined and the pipeline is ready. The **service** emits `session_end` (in `LeaveRoom` and `OnDisconnect`) because those flows are owned by the service layer, not the pipeline. `session_error` is emitted within `runHalf` when a stage fails.

**Design decision — `slog.LogAttrs` vs `slog.Info`**: Use `slog.LogAttrs` for all structured events. It is more efficient than `slog.Info(msg, key, val, ...)` because it avoids the `any` boxing of key-value pairs. For events with many fields (like `session_start` with 6+ fields), this matters.

#### Modified `runHalf` — instrument 5 stages

```go
func (s *Service) runHalf(ctx context.Context, p *pipeline, half pipelineHalf) {
    defer p.wg.Done()
    defer func() {
        // Log session_error for any unhandled panics if needed
    }()

    tracker := half.tracker
    roomID := p.roomID

    // ── Stage 1: Capture ──
    tracker.StartStage(StageCapture)

    opusCh := make(chan []byte, 8)
    err := s.peer.OnAudioTrack(ctx, half.sourceSessID, func(trackCh <-chan []byte) {
        for frame := range trackCh {
            // End capture when frame arrives from trackCh
            tracker.EndStage(StageCapture)
            select {
            case opusCh <- frame:
            case <-ctx.Done():
                return
            }
            // Start next capture for next frame
            // (the next frame's capture starts when the next frame arrives)
        }
        close(opusCh)
    })
    if err != nil {
        s.notifier.NotifySession(half.sourceSessID, "error",
            map[string]string{"reason": fmt.Sprintf("audio track setup failed: %v", err)})
        return
    }

    // ── Stage 2: Decode ──
    tracker.StartStage(StageDecode)
    pcmCh, err := s.codec.Decode(ctx, opusCh)
    tracker.EndStage(StageDecode)
    if err != nil {
        // Error recorded — register as error chunk
        half.errorChunks.Add(1)
        s.notifier.NotifySession(half.sourceSessID, "error",
            map[string]string{"reason": fmt.Sprintf("audio decode failed: %v", err)})
        tracker.Emit(ctx, half.dir, roomID, "error")
        return
    }

    // Buffer bridge (unchanged)
    bpCh := make(chan []byte, 1)
    p.wg.Add(1)
    go func() {
        defer p.wg.Done()
        defer close(bpCh)
        for {
            select {
            case frame, ok := <-pcmCh:
                if !ok { return }
                drainOldest(bpCh, frame)
            case <-ctx.Done():
                return
            }
        }
    }()

    // ── Stage 3: Translate ──
    tracker.StartStage(StageTranslate)
    translatedCh, err := s.translator.TranslateStream(ctx, bpCh, half.sourceLang, half.targetLang)
    tracker.EndStage(StageTranslate)
    if err != nil {
        half.errorChunks.Add(1)
        s.logSessionError(half.sourceSessID, "translate", err)
        s.notifier.NotifySession(half.sourceSessID, "error",
            map[string]string{"reason": fmt.Sprintf("translation failed: %v", err)})
        tracker.Emit(ctx, half.dir, roomID, "error")
        return
    }

    // ── Stage 4: Encode ──
    tracker.StartStage(StageEncode)
    opusOutCh, err := s.codec.Encode(ctx, translatedCh)
    tracker.EndStage(StageEncode)
    if err != nil {
        half.errorChunks.Add(1)
        s.logSessionError(half.targetSessID, "encode", err)
        s.notifier.NotifySession(half.targetSessID, "error",
            map[string]string{"reason": fmt.Sprintf("audio encode failed: %v", err)})
        tracker.Emit(ctx, half.dir, roomID, "error")
        return
    }

    // ── Stage 5: Send ──
    // SendAudio is streaming — each frame processed from opusOutCh gets its own chunk.
    for translatedFrame := range opusOutCh {
        tracker.StartStage(StageSend)
        err := s.peer.SendAudio(ctx, half.targetSessID, opusOutCh)
        // Note: the actual send is per-frame; we track the send stage for each chunk.
        // SendAudio may block until the frame is sent.

        // Actually, SendAudio takes the channel, not individual frames.
        // So the send stage wraps the entire SendAudio call.
    }

    // ABOVE IS CONCEPTUAL — the actual per-chunk loop needs restructuring.
    // See Section 1.2.1 for the per-chunk loop redesign.
}
```

#### 1.2.1 Per-chunk loop redesign

The current `runHalf` does NOT have a per-chunk loop — it calls pipeline stages sequentially and each stage returns a channel. Audio frames flow through channels. This means "capture" is triggered per frame, not "one chunk = one call to each stage".

**Design decision**: The pipeline stages are already channel-based. Each audio frame IS a chunk. The instrumentation wraps the channel-processing goroutines:

1. **Capture**: Times when a frame arrives from `trackCh` (the `OnAudioTrack` callback). Start capture when we receive a frame, end it immediately (the capture time ≈ 0 since it's already decoded by Pion by the time we get it).

2. **Decode** → **Translate** → **Encode** → **Send**: These return channels that process frames continuously. We instrument each stage's goroutine that reads from the input channel and writes to the output channel.

**Actual implementation approach**:

Rather than adding timestamps inside every stage callback (which would require modifying every adapter interface), we wrap the **channels** between stages:

```go
func (s *Service) runHalf(ctx context.Context, p *pipeline, half pipelineHalf) {
    defer p.wg.Done()

    tracker := half.tracker
    roomID := p.roomID
    var chunkErr error

    opusCh := make(chan []byte, 8)
    err := s.peer.OnAudioTrack(ctx, half.sourceSessID, func(trackCh <-chan []byte) {
        for frame := range trackCh {
            tracker.Reset() // new chunk starts
            tracker.StartStage(StageCapture)
            // Capture ends immediately — the frame is already in memory
            tracker.EndStage(StageCapture)

            tracker.StartStage(StageDecode)
            select {
            case opusCh <- frame:
            case <-ctx.Done():
                return
            }
            // Decode ends when opusCh is read — tracked in the decode loop below
            // (We end it in the goroutine that reads from opusCh)
        }
        close(opusCh)
    })
    if err != nil {
        s.notifier.NotifySession(...)
        return
    }

    // Decode loop — each PCM frame completes the decode stage
    pcmCh, err := s.codec.Decode(ctx, opusCh)
    if err != nil {
        half.errorChunks.Add(1)
        tracker.Emit(ctx, half.dir, roomID, "error")
        return
    }

    // Wrapping decode output: track decode end, start translate
    wrappedPCMCh := make(chan []byte, 1)
    p.wg.Add(1)
    go func() {
        defer p.wg.Done()
        defer close(wrappedPCMCh)
        for pcm := range pcmCh {
            tracker.EndStage(StageDecode)    // decode done
            tracker.StartStage(StageTranslate)
            select {
            case wrappedPCMCh <- pcm:
            case <-ctx.Done():
                return
            }
        }
    }()

    // Buffer bridge (drainOldest)
    bpCh := make(chan []byte, 1)
    p.wg.Add(1)
    go func() {
        defer p.wg.Done()
        defer close(bpCh)
        for {
            select {
            case frame, ok := <-wrappedPCMCh:
                if !ok { return }
                drainOldest(bpCh, frame)
            case <-ctx.Done():
                return
            }
        }
    }()

    // Translate
    translatedCh, err := s.translator.TranslateStream(ctx, bpCh, half.sourceLang, half.targetLang)
    if err != nil {
        half.errorChunks.Add(1)
        tracker.Emit(ctx, half.dir, roomID, "error")
        return
    }

    // Wrap translate output: end translate, start encode
    wrappedTranslatedCh := make(chan []byte, 1)
    p.wg.Add(1)
    go func() {
        defer p.wg.Done()
        defer close(wrappedTranslatedCh)
        for frame := range translatedCh {
            tracker.EndStage(StageTranslate)   // translate done
            tracker.StartStage(StageEncode)
            select {
            case wrappedTranslatedCh <- frame:
            case <-ctx.Done():
                return
            }
        }
    }()

    // Encode
    opusOutCh, err := s.codec.Encode(ctx, wrappedTranslatedCh)
    if err != nil {
        half.errorChunks.Add(1)
        tracker.Emit(ctx, half.dir, roomID, "error")
        return
    }

    // Wrap encode output: end encode, start send
    wrappedOpusOutCh := make(chan []byte, 1)
    p.wg.Add(1)
    go func() {
        defer p.wg.Done()
        defer close(wrappedOpusOutCh)
        for frame := range opusOutCh {
            tracker.EndStage(StageEncode)      // encode done
            tracker.StartStage(StageSend)
            select {
            case wrappedOpusOutCh <- frame:
            case <-ctx.Done():
                return
            }
        }
    }()

    // Send — final stage
    // SendAudio takes a channel and sends each frame. After each send, we
    // end the send stage and emit the chunk latency.
    // HOWEVER, the current SendAudio signature takes a channel and processes
    // all frames. We need a per-frame send.
    //
    // Decision: We wrap this in a goroutine that reads from wrappedOpusOutCh
    // and calls a per-frame send (or we accept that send latency is aggregated
    // across all frames).
    //
    // For the MVP, we treat SendAudio as a single stage that ends when the
    // channel is closed. Per-frame send tracking is future work.
    tracker.EndStage(StageCapture) // Capture already ended above — this is a no-op
    // ... etc.
}
```

**CRITICAL SIMPLIFICATION**: The above approach adds 3 extra goroutines and channels per pipeline half just for wrapping. This is unnecessary complexity.

**FINAL DECISION — Simplified approach**: Instrument at the **stage entry/exit points** where channels are created/returned, NOT inside per-frame goroutines. The semantics are:

| Stage | Start | End | Granularity |
|-------|-------|-----|-------------|
| `capture` | Frame received in `trackCh` callback | Immediately after (capture is instant — frame is in memory) | Per-frame |
| `decode` | Before `s.codec.Decode(ctx, opusCh)` | After `Decode` returns (channel ready) | Per-call |
| `translate` | Before `s.translator.TranslateStream(ctx, bpCh, ...)` | After `TranslateStream` returns (channel ready) | Per-call |
| `encode` | Before `s.codec.Encode(ctx, translatedCh)` | After `Encode` returns (channel ready) | Per-call |
| `send` | Before `s.peer.SendAudio(ctx, ...)` | After `SendAudio` returns (or error) | Per-call |

This means each pipeline stage's timing covers the **entire streaming operation** (from when the channel is created to when it completes). The `total_ms` for a chunk is the sum of all stage durations for that "wave" of processing.

This is MUCH simpler — no extra goroutines. The `tracker.Reset()` is called at the top of each frame in the `trackCh` callback, and `tracker.Emit()` is called after each stage completes (or on error).

Wait — but with channel-based streaming, "decode" runs continuously. There's no single start/end. Let me think about this differently.

**REVISED FINAL DECISION**: Accept that with channel-based streaming, "per-chunk" timing is approximate. The actual design:

1. **One `tracker.Reset()` per audio frame** received in `trackCh`.
2. **Capture** timing: from frame arrival to frame being sent to `opusCh` (≈instant).
3. **Decode/Translate/Encode/Send** timings: measure the full duration of the `Decode()`/`TranslateStream()`/`Encode()`/`SendAudio()` calls. These are NOT per-frame — they cover the entire streaming lifecycle.
4. **Simplest useful metric**: `total_ms` from first frame to last frame completion, divided by number of chunks. This gives average latency per chunk.

**ABSOLUTELY FINAL DESIGN — Per-chunk timestamps in the callback**:

The `SendAudio` function signature determines the architecture. Looking at the interface:

```go
// from driven.WebRTCPeer
SendAudio(ctx context.Context, sessionID string, frames <-chan []byte) error
```

This takes a channel — it processes all frames. So "send" is one measurement per pipeline half.

Given the streaming nature, the pragmatic approach is:

```go
// runHalf — simplified instrumentation
func (s *Service) runHalf(ctx context.Context, p *pipeline, half pipelineHalf) {
    defer p.wg.Done()

    tracker := half.tracker
    roomID := p.roomID
    frameCount := 0

    opusCh := make(chan []byte, 8)
    err := s.peer.OnAudioTrack(ctx, half.sourceSessID, func(trackCh <-chan []byte) {
        for frame := range trackCh {
            frameCount++
            tracker.Reset()                     // ← new chunk
            tracker.StartStage(StageCapture)
            // capture is instant
            tracker.EndStage(StageCapture)

            select {
            case opusCh <- frame:
            case <-ctx.Done():
                return
            }
        }
        close(opusCh)
    })
    if err != nil {
        notifyError(...)
        return
    }

    // Decode — timing covers the entire decode call
    tracker.StartStage(StageDecode)
    pcmCh, err := s.codec.Decode(ctx, opusCh)
    if err != nil {
        half.errorChunks.Add(int64(frameCount))
        tracker.Emit(ctx, half.dir, roomID, "error")
        return
    }
    tracker.EndStage(StageDecode)

    // Buffer bridge (unchanged)
    bpCh := make(chan []byte, 1)
    p.wg.Add(1)
    go func() { /* drainOldest loop */ }()

    tracker.StartStage(StageTranslate)
    translatedCh, err := s.translator.TranslateStream(ctx, bpCh, half.sourceLang, half.targetLang)
    if err != nil {
        half.errorChunks.Add(int64(frameCount))
        tracker.Emit(ctx, half.dir, roomID, "error")
        return
    }
    tracker.EndStage(StageTranslate)

    tracker.StartStage(StageEncode)
    opusOutCh, err := s.codec.Encode(ctx, translatedCh)
    if err != nil {
        half.errorChunks.Add(int64(frameCount))
        tracker.Emit(ctx, half.dir, roomID, "error")
        return
    }
    tracker.EndStage(StageEncode)

    tracker.StartStage(StageSend)
    if err := s.peer.SendAudio(ctx, half.targetSessID, opusOutCh); err != nil {
        half.errorChunks.Add(int64(frameCount))
        tracker.Emit(ctx, half.dir, roomID, "error")
        return
    }
    tracker.EndStage(StageSend)

    // All stages completed successfully
    half.totalChunks.Add(int64(frameCount))
    tracker.Emit(ctx, half.dir, roomID, "ok")
}
```

This means the `chunk_latency` log event is emitted once per pipeline half, with one measurement per stage covering the entire streaming session. The `chunk_id` is the total frame count.

**Why this is acceptable**: The proposal and spec both say "timestamps in each stage of runHalf". The channel-based streaming means stages run concurrently (pipeline parallelism), so individual stage timings are the wall-clock time for that stage. The `total_ms` is NOT the sum of stages (they overlap) — it's the total wall-clock time from first frame to last frame.

For the MVP, this gives us useful aggregate data: which stage takes the longest (translate), what's the total pipeline latency, and how many frames are processed.

**Future improvement** (Sprint 5+): If per-frame timing is needed, refactor `SendAudio` to accept individual frames and wrap each stage's channel with a timed goroutine. This requires interface changes to `driven.WebRTCPeer`.

---

### 1.3 Session Events — emitter ownership

| Event | Emitter | Where | Fields |
|-------|---------|-------|--------|
| `session_start` | `Service.startPipeline()` | After `JoinRoom` completes for both peers | `room_id`, `session_id`, `user_id`, `lang`, `component: "service"` |
| `session_end` | `Service.LeaveRoom()` and `Service.OnDisconnect()` | Before cleanup in voluntary leave; in disconnect flow | `session_id`, `duration_sec`, `reason` ("voluntary"/"disconnect"/"timeout"), `component: "service"` |
| `session_error` | `Service.runHalf()` | When a pipeline stage returns error | `session_id`, `error`, `error_count`, `stage`, `component: "pipeline"` |
| `pipeline_start` | `Service.startPipeline()` | Before launching goroutines | `room_id`, `sessA`, `sessB`, `langA`, `langB`, `component: "pipeline"` |
| `pipeline_stop` | Goroutine in `startPipeline()` | After both `runHalf` goroutines complete (p.wg.Wait()) | `room_id`, `total_chunks_AtoB`, `total_chunks_BtoA`, `component: "pipeline"` |

#### `session_end` in `LeaveRoom`

```go
func (s *Service) LeaveRoom(ctx context.Context, roomID, userID string) error {
    // ... existing lookup ...

    // Emit session_end before cleanup
    slog.LogAttrs(ctx, slog.LevelInfo, "session_event",
        slog.String("event", "session_end"),
        slog.String("session_id", sessID),
        slog.Int64("duration_sec", int64(time.Since(sess.CreatedAt).Seconds())),
        slog.String("reason", "voluntary"),
        slog.String("component", "service"),
    )

    // ... rest of cleanup ...
}
```

#### `session_end` in `OnDisconnect`

```go
func (s *Service) OnDisconnect(ctx context.Context, sessionID string) error {
    // ... existing lookup ...

    // Emit session_end for the disconnecting peer
    slog.LogAttrs(ctx, slog.LevelInfo, "session_event",
        slog.String("event", "session_end"),
        slog.String("session_id", sessionID),
        slog.Int64("duration_sec", int64(time.Since(sess.CreatedAt).Seconds())),
        slog.String("reason", "disconnect"),
        slog.String("component", "service"),
    )

    // ... rest of grace timer logic ...
}
```

#### `session_end` with reason `"timeout"`

In the grace timer callback (inside `OnDisconnect`), when the timer fires and calls `DeleteRoom`:

```go
time.AfterFunc(s.cfg.GracePeriod, func() {
    slog.LogAttrs(ctx, slog.LevelInfo, "session_event",
        slog.String("event", "session_end"),
        slog.String("session_id", sessionID),
        slog.Int64("duration_sec", int64(time.Since(sess.CreatedAt).Seconds())),
        slog.String("reason", "timeout"),
        slog.String("component", "service"),
    )
    // ... DeleteRoom ...
})
```

#### `session_error` helper

```go
// logSessionError emits a session_error event and tracks error count.
func (s *Service) logSessionError(sessionID, stage string, err error) {
    s.mu.RLock()
    sess, ok := s.sessions[sessionID]
    s.mu.RUnlock()
    if !ok {
        return
    }
    sess.ErrorCount++ // add ErrorCount field to session.Session
    slog.LogAttrs(context.Background(), slog.LevelError, "session_event",
        slog.String("event", "session_error"),
        slog.String("session_id", sessionID),
        slog.String("error", err.Error()),
        slog.Int("error_count", sess.ErrorCount),
        slog.String("stage", stage),
        slog.String("component", "pipeline"),
    )
}
```

Requires adding `ErrorCount int` to `session.Session`:

```go
// internal/domain/session/session.go
type Session struct {
    ID         string
    RoomID     string
    UserID     string
    Lang       string
    CreatedAt  time.Time
    ErrorCount int // NEW: incremented on pipeline errors
}
```

---

### 1.4 Logging Modernization

#### 1.4.1 `cmd/server/main.go` — JSON handler + `-log-level` flag

```go
var logLevel = flag.String("log-level", "info", "log level (debug, info, warn, error)")

func run() int {
    flag.Parse()

    var level slog.Level
    switch *logLevel {
    case "debug":
        level = slog.LevelDebug
    case "info":
        level = slog.LevelInfo
    case "warn":
        level = slog.LevelWarn
    case "error":
        level = slog.LevelError
    default:
        level = slog.LevelInfo
    }

    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: level,
    }))
    slog.SetDefault(logger)

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-quit
        slog.Info("shutdown_starting", "component", "main")
        cancel()
    }()

    // ... driven adapters ...

    svc, err := roomsvc.NewService(cfg, repo, peer, tr, codec, hub)
    if err != nil {
        slog.Error("service_creation_failed", "component", "main", slog.Any("err", err))
        return 1  // was os.Exit(1) — fix for WARN-03
    }

    // ... wire hub, start sweeper, start server ...

    slog.Info("server_starting", "component", "main", "addr", srv.Addr())
    if err := srv.ListenAndServe(ctx); err != nil {
        slog.Error("server_error", "component", "http", slog.Any("err", err))
        return 1
    }

    return 0
}
```

**Fix for WARN-03**: `os.Exit(1)` → `return 1` on lines 64 and 82 (now fixed above). `main()` calls `os.Exit(run())`, so `return 1` achieves the same effect while allowing `defer cancel()` to execute.

#### 1.4.2 All slog call sites — standardized format

Every `slog` call in the modified files MUST follow this pattern:

```
slog.Level("snake_case_identifier",
    "component", "<standard_value>",
    // ... context fields as key-value pairs ...
    slog.Any("err", err),  // if error
)
```

**Exact changes per file**:

##### `cmd/server/main.go`

| Line (current) | Current | After |
|----------------|---------|-------|
| 24 | `slog.New(slog.NewTextHandler(os.Stdout, nil))` | `slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))` |
| 35 | `slog.Info("shutdown signal received")` | `slog.Info("shutdown_starting", "component", "main")` |
| 63 | `slog.Error("creating service", slog.Any("err", err))` | `slog.Error("service_creation_failed", "component", "main", slog.Any("err", err))` |
| 79 | `slog.Info("TalkGo starting")` | `slog.Info("server_starting", "component", "main", "addr", s.cfg.Addr)` |
| 81 | `slog.Error("server error", slog.Any("err", err))` | `slog.Error("server_error", "component", "http", slog.Any("err", err))` |

##### `internal/adapters/signaling/hub.go`

| Line (current) | Current | After |
|----------------|---------|-------|
| 146-148 | `slog.Error("hub: OnDisconnect", slog.String("sessionID", sessionID), slog.Any("err", err))` | `slog.Error("on_disconnect_error", "component", "hub", "session_id", sessionID, slog.Any("err", err))` |
| 168 | `slog.Error("websocket upgrade", slog.Any("err", err))` | `slog.Error("ws_upgrade_failed", "component", "hub", slog.Any("err", err))` |
| 217 | `slog.Error("marshalling signaling response", slog.Any("err", err))` | `slog.Error("signal_response_marshal_error", "component", "hub", slog.Any("err", err))` |
| 248 | `slog.Error("NotifySession marshal", slog.Any("err", err))` | `slog.Error("notify_session_marshal_error", "component", "hub", slog.Any("err", err))` |

##### `internal/adapters/signaling/client.go`

| Line (current) | Current | After |
|----------------|---------|-------|
| 47 | `slog.Error("websocket write", slog.Any("err", err))` | `slog.Error("ws_write_error", "component", "hub", slog.Any("err", err))` |
| 77 | `slog.Error("websocket read", slog.Any("err", err))` | `slog.Error("ws_read_error", "component", "hub", slog.Any("err", err))` |

##### `internal/adapters/http/server.go`

| Line (current) | Current | After |
|----------------|---------|-------|
| 83 | `slog.Info("server starting", slog.String("addr", s.cfg.Addr))` | `slog.Info("http_listening", "component", "http", "addr", s.cfg.Addr)` |
| 85 | `slog.Error("server error", slog.Any("err", err))` | (removed — handled by main.go) |
| 90 | `slog.Info("shutting down server")` | `slog.Info("http_shutdown", "component", "http")` |
| 98 | `slog.Info("server stopped")` | `slog.Info("http_stopped", "component", "http")` |
| 134 | `slog.Error("createRoom", slog.Any("err", err))` | `slog.Error("create_room_error", "component", "http", slog.Any("err", err))` |
| 152 | `slog.Error("deleteRoom", slog.Any("err", err))` | `slog.Error("delete_room_error", "component", "http", slog.Any("err", err))` |
| 178 | `slog.Error("getRoomByShortCode", slog.Any("err", err))` | `slog.Error("find_by_code_error", "component", "http", slog.Any("err", err))` |
| 200 | `slog.Error("wsHandler roomExists", slog.Any("err", err))` | `slog.Error("ws_handler_error", "component", "http", slog.Any("err", err))` |

##### `internal/app/roomsvc/service.go`

| Line (current) | Current | After |
|----------------|---------|-------|
| 138-140 | `slog.Error("closing peer session on room delete", ...)` | `slog.Error("close_session_error", "component", "service", "session_id", sessID, slog.Any("err", err))` |
| 282-284 | `slog.Error("closing peer session", ...)` | `slog.Error("close_session_error", "component", "service", "session_id", sessID, slog.Any("err", err))` |
| 370-373 | `slog.Error("grace timer: deleting room", ...)` | `slog.Error("grace_timer_delete_error", "component", "service", "room_id", roomID, slog.Any("err", err))` |
| 406 | `slog.Error("expiration sweep: listing expired rooms", ...)` | `slog.Error("sweep_list_error", "component", "service", slog.Any("err", err))` |
| 411-413 | `slog.Error("expiration sweep: deleting room", ...)` | `slog.Error("sweep_delete_error", "component", "service", "room_id", r.ID, slog.Any("err", err))` |

#### 1.4.3 Standard slog field names

| Field | Type | When to include |
|-------|------|-----------------|
| `component` | `string` | ALWAYS — one of: `main`, `http`, `hub`, `service`, `pipeline`, `loadgen` |
| `room_id` | `string` | When room context is available |
| `session_id` | `string` | When session context is available |
| `duration_ms` | `int64` | When an operation is measured |
| `err` | `error` | On error events (use `slog.Any("err", err)`) |

---

### 1.5 Bug Fixes Sprint 3

#### 1.5.1 CRIT-01: copylocks in `ListExpired`

**Current state** (post Sprint 3 partial fix): The port interface and implementation already return `[]*room.Room`. However, the `golangci-lint` copylocks check still fires because somewhere in the call chain a value copy occurs.

**Fix**: Audit ALL callers of `ListExpired`:

| File | Line | Current | Fix |
|------|------|---------|-----|
| `internal/ports/driven/room_repository.go` | 37 | `ListExpired(...) ([]*room.Room, error)` | ✅ Already correct |
| `internal/app/roomsvc/repository.go` | 95-105 | Returns `expired []*room.Room` | ✅ Already correct |
| `internal/app/roomsvc/repository_test.go` | 184-233 | Uses `repo.ListExpired(...)` | ✅ Already works with pointers |
| `internal/app/roomsvc/service.go` | 409 | `for _, r := range expired` (r is `*room.Room`) | ✅ Already correct |
| `internal/app/roomsvc/service_sprint3_test.go` | 283 | `return []*room.Room{r}, nil` | ✅ Already correct |
| `internal/ports/driven/mocks/mock_room_repository.go` | 86 | Returns `[]*room.Room` | ✅ Already correct |

**If the lint still fires**: The problem may be in the `Room` struct itself — `sync.Mutex` embedded as a value field inside `Room` triggers copylocks when `Room` is passed by value anywhere. Check that ALL code paths pass `*room.Room` (pointer), never `room.Room` (value). The `append(expired, rm)` in repository.go line 101 appends the pointer correctly.

**Preventive**: Run `golangci-lint run ./...` and fix any remaining copylocks anywhere in the codebase.

#### 1.5.2 WARN-01: gofmt in 5 files

Run `gofmt -w` on:
1. `internal/adapters/signaling/hub.go`
2. `internal/ports/driven/mocks/mock_room_repository.go`
3. `internal/adapters/http/server_test.go`
4. `internal/app/roomsvc/service_sprint3_test.go`
5. `internal/domain/room/room.go`

Then verify: `go fmt ./...` produces zero diff.

#### 1.5.3 WARN-03: exitAfterDefer

**Where**: `cmd/server/main.go` lines 64 and 82.

**Fix**: Replace both `os.Exit(1)` with `return 1`. The `main()` function already uses `os.Exit(run())`, so `return 1` from `run()` achieves the same exit code but allows all deferred functions (including `cancel()`) to execute.

#### 1.5.4 WARN-06: TestHub_PeerLeft_NotifiedOnDisconnect

**Where**: `internal/adapters/signaling/hub_test.go` (the test that checks peer-left notification on disconnect).

**Fix**: Change the assertion from `t.Logf` to `t.Errorf` when `peer-left` is not received within the timeout. The test already tracks `roomClients` properly, so not receiving `peer-left` indicates a real bug.

---

### 1.6 Testing - Instrumentation

#### `latency.go` tests (new: `latency_test.go`)

| Test | Scenario |
|------|----------|
| `TestLatencyTracker_StartEndStages` | StartStage → EndStage produces correct Duration |
| `TestLatencyTracker_EmitLogsCorrectFields` | Emit produces JSON with all expected fields |
| `TestLatencyTracker_ChunkIDIncrements` | Reset increments chunkID, independent per tracker |
| `TestLatencyTracker_PoolReusesInstances` | sync.Pool returns same pointer after Emit+Reset |
| `BenchmarkLatencyTracker_Allocations` | AllocsPerRun < 3 per chunk |
| `TestLatencyTracker_ConcurrentSafety` | Two trackers operate independently in parallel |

#### `pipeline_test.go` (new or modified)

| Test | Scenario |
|------|----------|
| `TestPipeline_EmitChunkLatencyLog` | Pipeline with mock stages emits chunk_latency with correct fields |
| `TestPipeline_ErrorChunkLogged` | Failing stage produces chunk_latency with status:"error" |
| `TestPipeline_Counters` | TotalChunks and ErrorChunks increment correctly |

**Test helper pattern for log capture**:

```go
func newLogCapture(t *testing.T) (*bytes.Buffer, *slog.Logger) {
    t.Helper()
    var buf bytes.Buffer
    logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
    return &buf, logger
}
```

Tests temporarily replace the default logger:
```go
old := slog.Default()
slog.SetDefault(testLogger)
defer slog.SetDefault(old)
```

---

## Section 2: Workstream B — Network Testing

### 2.1 Loadgen Peer (`cmd/loadgen/main.go`)

**Design decision — WebSocket-only MVP**: The loadgen peer does NOT implement full WebRTC. It connects via WebSocket, follows the signaling protocol (join → offer → answer → ICE), but uses a mock audio source (synthetic PCM tone) and measures signaling round-trip time. Full WebRTC audio pipeline testing is deferred to Sprint 5.

**Why**: Implementing a WebRTC peer in Go requires Pion and significant complexity (ICE, DTLS, SRTP, codec negotiation). A WS-only peer can be built in ~150 lines of Go and gives us 80% of the value: signaling latency measurement, server stability under load, and network condition testing.

**Architecture**:

```
cmd/loadgen/
├── main.go          # CLI entry point, flag parsing
├── session.go       # WebSocket connection + signaling protocol
├── audio.go         # Synthetic PCM tone generator (sine wave, 24kHz, 20ms frames)
└── report.go        # JSON report generation (RTT stats, packet loss)
```

**CLI flags**:

```
-server string      URL del servidor (default "localhost:8080")
-room string        Room ID (default: auto-generado vía HTTP POST /rooms)
-lang string        Idioma del peer (default "es")
-duration duration  Duración de la sesión (default 30s)
-profile string     Perfil de red para documentar reporte (default "wifi-home")
-output string      Archivo de reporte (default: stdout JSON)
```

**Flow**:

1. Resolve server URL: if `-server` is `localhost:8080`, create WS URL `ws://localhost:8080/ws/<roomID>`.
2. If `-room` is empty, create a room via HTTP POST `http://localhost:8080/rooms` with `{source_lang: "es", target_lang: "en"}`.
3. Open WebSocket to `ws://<server>/ws/<roomID>`.
4. Send `join` message: `{type: "join", user_id: "loadgen-<uuid>", room_id: "<roomID>", lang: "<lang>"}`.
5. Receive `joined` with `sessionID`.
6. Send `offer` with dummy SDP (empty string — server handles it).
7. Receive `answer`.
8. Send `ice-candidate` (dummy candidate).
9. Loop at 20ms intervals (50Hz) for duration:
   - Send a simple ping message (or use `ice-candidate` pings).
   - Measure RTT for each message.
10. After duration expires, send `leave`, close WS.
11. Calculate and emit JSON report to stdout.

**Report JSON structure**:

```json
{
  "profile": "4g",
  "duration_sec": 30,
  "total_messages": 150,
  "avg_rtt_ms": 120.5,
  "min_rtt_ms": 85,
  "max_rtt_ms": 450,
  "p50_rtt_ms": 115,
  "p90_rtt_ms": 200,
  "packet_loss_pct": 2.5,
  "errors": []
}
```

**RTT calculation**: RTT per message = time between send and response receipt. `packet_loss_pct` = (messages sent - responses received) / messages sent × 100. p50/p90 calculated by sorting RTT slice and taking the 50th/90th percentile.

**NFR-07/NFR-08 compliance**: `cmd/loadgen/` is a `package main` — it does NOT import any `internal/` packages. It uses only `gorilla/websocket` (already a dependency) and Go stdlib. It relies on the server's existing HTTP and WebSocket endpoints — no internal API.

---

### 2.2 Network Simulation Scripts (`scripts/network-test/`)

#### Structure

```
scripts/network-test/
├── simulate-4g.ps1              # Windows: netsh-based network conditioning
├── simulate-4g.sh               # Linux: tc/netem-based network conditioning
├── run-test-session.sh           # Full automation: profile → server → loadgen → parse → report
├── configs/
│   ├── 4g.yml                    # 10Mbps, 100ms RTT, 5% loss, 10ms jitter
│   ├── wifi-cafe.yml             # 5Mbps, 150ms RTT, 8% loss, 20ms jitter
│   ├── wifi-home.yml             # 50Mbps, 20ms RTT, 1% loss, 2ms jitter
│   └── wan-lossy.yml             # 2Mbps, 300ms RTT, 15% loss, 30ms jitter
└── README.md                     # How to use, what to test, expected metrics
```

#### `simulate-4g.ps1` (Windows)

```powershell
param(
    [string]$Profile,
    [int]$Bandwidth,
    [int]$LatencyMs,
    [int]$LossPct,
    [switch]$Reset
)

# Requires Admin
# Requires -RunAsAdministrator check at top

if ($Reset) {
    # netsh int tcp set global autotuninglevel=normal
    # Remove any prior restrictions
    Write-Host "Network restrictions removed"
    exit 0
}

if ($Profile) {
    # Parse configs/$Profile.yml
}

# netsh int tcp set global autotuninglevel=disabled
# netsh advfirewall ... (apply bandwidth/rate limits)
# Note: Windows netsh does NOT support packet loss or latency directly
# We use bandwidth throttling only
```

**Design decision**: Windows `netsh` cannot simulate packet loss or latency — those are Linux `tc netem` features. The Windows script applies bandwidth throttling only (via `netsh` TCP autotuning and advance firewall rate limits). For full 4G simulation, use Linux with `simulate-4g.sh`.

#### `simulate-4g.sh` (Linux)

```bash
#!/usr/bin/env bash
# Usage: ./simulate-4g.sh -Profile 4g
#        ./simulate-4g.sh -Interface wlan0 -LatencyMs 150 -LossPct 3 -RateMbps 5
#        ./simulate-4g.sh -Reset

while [[ $# -gt 0 ]]; do
    case $1 in
        -Profile) PROFILE="$2"; shift 2 ;;
        -Interface) IFACE="$2"; shift 2 ;;
        -LatencyMs) LATENCY="$2"; shift 2 ;;
        -LossPct) LOSS="$2"; shift 2 ;;
        -RateMbps) RATE="$2"; shift 2 ;;
        -Reset) RESET=1; shift ;;
    esac
done

[[ -n "$RESET" ]] && {
    tc qdisc del dev "$IFACE" root 2>/dev/null
    echo "Reset complete"
    exit 0
}

[[ -n "$PROFILE" ]] && {
    # Parse configs/$PROFILE.yml for values
}

# Apply tc rules
tc qdisc add dev "$IFACE" root handle 1: htb default 30
tc class add dev "$IFACE" parent 1: classid 1:1 htb rate "${RATE}mbit"
tc qdisc add dev "$IFACE" parent 1:1 netem delay "${LATENCY}ms" loss "${LOSS}%" 25%
```

**Design decision**: YAML parsing in bash is done with a simple `grep` + `awk` pipeline, NOT with Python or any external YAML parser. Linux systems always have `grep` and `awk`. The config files have a simple flat format.

```bash
# Parse YAML value (simple flat YAML)
parse_yaml_value() {
    local key="$1" file="$2"
    grep "^${key}:" "$file" | awk '{print $2}'
}

BANDWIDTH=$(parse_yaml_value "bandwidth_mbps" "$PROFILE_FILE")
```

#### `run-test-session.sh`

```bash
#!/usr/bin/env bash
# Usage: ./run-test-session.sh -Profile wifi-home -Duration 10s

# 1. Parse args
# 2. Apply network profile (if not -SkipSimulation)
# 3. go run ./cmd/server & — capture PID
# 4. sleep 2 (wait for server)
# 5. go run ./cmd/loadgen -server localhost:8080 -duration "$DURATION" -profile "$PROFILE" > loadgen-report.json
# 6. Parse server logs with jq
#    cat server.log | jq 'select(.msg == "chunk_latency") | .total_ms' | sort -n | ...
# 7. Generate consolidated report
# 8. Kill server, reset network
# 9. Output JSON report
```

**Report status logic**:

| Condition | Status |
|-----------|--------|
| Loadgen connection failed | `"failed"` |
| error_rate_pct > 15% or latency_p90 > 2500ms | `"failed"` |
| error_rate_pct 5-15% or latency_p90 1500-2500ms | `"degraded"` |
| Otherwise | `"ok"` |

---

### 2.3 Network Profiles (`scripts/network-test/configs/`)

Each `.yml` file:

```yaml
name: "4g"
description: "4G móvil estándar — 100ms RTT, 5% pérdida, 10Mbps"
bandwidth_mbps: 10
rtt_ms: 100
loss_pct: 5
jitter_ms: 10
interface: "eth0"
```

| File | bandwidth_mbps | rtt_ms | loss_pct | jitter_ms |
|------|:-------------:|:------:|:--------:|:---------:|
| `4g.yml` | 10 | 100 | 5 | 10 |
| `wifi-cafe.yml` | 5 | 150 | 8 | 20 |
| `wifi-home.yml` | 50 | 20 | 1 | 2 |
| `wan-lossy.yml` | 2 | 300 | 15 | 30 |

---

## Section 3: Workstream C — Onboarding Documentation

### 3.1 README Expansion

The `README.md` is expanded at the END of the sprint (after Workstream A is stable). Sections to add:

1. **Prerequisites** — Go 1.23+, Node.js 20+, React Native CLI, Android SDK, Xcode, OpenAI API key
2. **Quick Start** — `git clone`, `make setup`, configure `.env`, `go run ./cmd/server`
3. **Architecture Overview** — Hexagonal layers diagram (ASCII or Mermaid):
   ```
   Driving Ports (HTTP/WS) → Service Layer (roomsvc) → Driven Ports (WebRTC/Translator/Codec/Repo)
   ```
4. **Make Targets** — `make test`, `make lint`, `make run`, `make build`
5. **Running Tests** — `go test -race ./...`, `go test -cover ./...`
6. **Logging** — How to read JSON logs, filtering with `jq`
7. **Network Testing** — How to run `run-test-session.sh`
8. **Troubleshooting** — Common issues (Pion ICE failures, OpenAI API key, CGO)

---

## Section 4: File Change Table

| File | Action | What Changes |
|------|--------|-------------|
| `internal/app/roomsvc/latency.go` | **create** | `LatencyTracker`, `ChunkLatency`, `StageTiming`, stage name constants, `NewLatencyTracker`, `StartStage`, `EndStage`, `Emit`, `Reset` |
| `internal/app/roomsvc/latency_test.go` | **create** | Unit tests for tracker: stage timing, emit, pool reuse, allocs, concurrency |
| `internal/app/roomsvc/pipeline.go` | modify | `pipelineHalf` +tracker, +totalChunks/errorChunks atomic, +dir field; `startPipeline` emits session_start/pipeline_start + pipeline_stop on completion; `runHalf` instrumented with 5 stage timings |
| `internal/app/roomsvc/pipeline_test.go` | **create** | Pipeline integration tests with log capture: chunk_latency fields, error status, counters |
| `internal/app/roomsvc/service.go` | modify | `LeaveRoom` emits session_end; `OnDisconnect` emits session_end (disconnect); grace timer emits session_end (timeout); all slog calls modernized to snake_case + component field; `sweepExpiredRooms` iterates `[]*room.Room` (already pointer) |
| `internal/app/roomsvc/service_test.go` | modify | Add test for session_end emission on LeaveRoom/OnDisconnect |
| `internal/domain/session/session.go` | modify | +`ErrorCount int` field (incremented on pipeline errors) |
| `internal/adapters/signaling/hub.go` | modify | All slog calls modernized to snake_case + component field; gofmt applied |
| `internal/adapters/signaling/client.go` | modify | All slog calls modernized to snake_case + component field |
| `internal/adapters/signaling/hub_test.go` | modify | WARN-06: `t.Logf` → `t.Errorf` in `TestHub_PeerLeft_NotifiedOnDisconnect` |
| `internal/adapters/http/server.go` | modify | All slog calls modernized to snake_case + component field |
| `internal/adapters/http/server_test.go` | modify | gofmt |
| `cmd/server/main.go` | modify | `slog.NewJSONHandler` replaces `NewTextHandler`; `-log-level` flag; `os.Exit(1)` → `return 1` (WARN-03); slog modernized |
| `internal/ports/driven/room_repository.go` | modify | Verify `ListExpired` returns `[]*room.Room` (verify CRIT-01 fix) |
| `internal/ports/driven/mocks/mock_room_repository.go` | modify | gofmt (WARN-01) |
| `internal/domain/room/room.go` | modify | gofmt (WARN-01) |
| `cmd/loadgen/main.go` | **create** | CLI entry point, flag parsing |
| `cmd/loadgen/session.go` | **create** | WebSocket connect, signaling protocol, RTT measurement |
| `cmd/loadgen/audio.go` | **create** | Synthetic PCM tone generator (sine, 24kHz, 20ms frames) |
| `cmd/loadgen/report.go` | **create** | JSON report with RTT stats, p50/p90, packet loss |
| `scripts/network-test/simulate-4g.ps1` | **create** | Windows netsh bandwidth throttling |
| `scripts/network-test/simulate-4g.sh` | **create** | Linux tc/netem network conditioning |
| `scripts/network-test/run-test-session.sh` | **create** | Full automation: profile → server → loadgen → report |
| `scripts/network-test/configs/4g.yml` | **create** | 10Mbps, 100ms, 5% loss |
| `scripts/network-test/configs/wifi-cafe.yml` | **create** | 5Mbps, 150ms, 8% loss |
| `scripts/network-test/configs/wifi-home.yml` | **create** | 50Mbps, 20ms, 1% loss |
| `scripts/network-test/configs/wan-lossy.yml` | **create** | 2Mbps, 300ms, 15% loss |
| `scripts/network-test/README.md` | **create** | Documentation for running network tests |
| `README.md` | modify | Expand with prerequisites, quick start, architecture, commands, testing, troubleshooting |

---

## Section 5: Design Decisions Summary

### D-01: LatencyTracker lives in `internal/app/roomsvc/latency.go` (NOT a new package)
**Rationale**: The tracker is tightly coupled to the pipeline. A separate package would add import boilerplate for zero reuse benefit. If the tracker becomes useful elsewhere, extract it later.

### D-02: ChunkLatency is a value struct, recycled via `sync.Pool`
**Rationale**: Value struct avoids heap escapes in the pool. The `sync.Pool` reuses the backing array for `StageTimings`, keeping allocations to < 3 per chunk. Benchmark target: zero allocations after warmup.

### D-03: Pipeline instrumentation is INLINE, not a wrapper
**Rationale**: A wrapper around `runHalf` would need to duplicate the full signature and error handling. Inline `StartStage()`/`EndStage()` calls are more readable and maintainable. The instrumentation adds < 20 lines to `runHalf`.

### D-04: Per-stage timing covers the FULL streaming call, not per-frame
**Rationale**: The channel-based pipeline means stages run concurrently. Each stage's timing (Decode, TranslateStream, Encode, SendAudio) covers the entire streaming lifecycle. Per-frame timing would require wrapping every channel — adding goroutines and complexity. For the MVP, aggregate timing per pipeline half is sufficient for identifying bottlenecks.

**Tradeoff**: We won't get per-frame jitter measurement with this approach. If needed in Sprint 5, we can add a channel wrapper with per-frame timestamps.

### D-05: Session events are emitted by BOTH service and pipeline
**Rationale**: The service owns session lifecycle (`LeaveRoom`, `OnDisconnect`) so `session_end` belongs there. The pipeline owns the translation lifecycle (`startPipeline`, `runHalf`) so `session_start`, `pipeline_start`, `pipeline_stop` belong there. Splitting by ownership keeps each component cohesive.

- **Service emits**: `session_end` (voluntary/disconnect/timeout)
- **Pipeline emits**: `session_start`, `pipeline_start`, `pipeline_stop`, `session_error`

### D-06: Loadgen is WebSocket-only (no WebRTC) for MVP
**Rationale**: Full WebRTC in Go requires Pion and substantial complexity (ICE, DTLS, SRTP/SCTP, codec negotiation). WebSocket-only covers 80% of the value: signaling latency, server stability, protocol compliance. Full WebRTC testing is deferred to Sprint 5.

**Tradeoff**: We won't measure audio pipeline latency from the loadgen. The server-side `chunk_latency` logs provide audio latency data; the loadgen measures signaling latency.

### D-07: Log messages are `snake_case` identifiers, NOT phrases
**Rationale**: `"chunk_latency"` is easier to grep, filter, and dashboard than `"Chunk latency measured"`. Standard identifiers enable log processing with `jq` (e.g., `jq 'select(.msg == "chunk_latency") | .total_ms'`). This is a hard convention: ALL `slog` calls in modified files MUST use snake_case identifiers.

### D-08: `slog.LogAttrs` preferred over `slog.Info` for multi-field events
**Rationale**: `slog.LogAttrs` avoids boxing each key-value pair in `any`. For events with 5+ fields (like `session_start` with 6 fields), this reduces allocations. For simple events (2-3 fields), `slog.Info` with inline args is acceptable.

### D-09: `totalChunks`/`errorChunks` are `atomic.Int64`
**Rationale**: These counters are written by the pipeline goroutine and read by the `pipeline_stop` goroutine (via `p.wg.Wait()`). `atomic.Int64` provides lock-free safe concurrent access. Go 1.23 supports `atomic.Int64` (added in Go 1.19).

### D-10: Windows network simulation is bandwidth-only (no loss/latency)
**Rationale**: `netsh` does not support packet loss or latency injection. These are Linux-only features via `tc netem`. The Windows script applies bandwidth throttling and documents this limitation. Full 4G simulation requires Linux or WSL2.

### D-11: Reconnection to session events is handled via `session_end` + new `session_start`
**Rationale**: When a peer disconnects and reconnects (grace timer cancels), the sequence is: `session_end` (disconnect) → `session_start` (reconnect). The `reconnect` is not a separate event — it's the same lifecycle with a new session_start. The `duration_sec` on session_start is reset for the new connection.

### D-12: No new Go dependencies
**Rationale**: All instrumentation uses `log/slog`, `time`, `sync`, `sync/atomic`, `encoding/json`, `fmt`, `strconv` — all stdlib. The loadgen uses the already-existing `gorilla/websocket` dependency. No new `go.mod` entries.
