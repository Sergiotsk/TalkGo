package roomsvc

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"
)

// Stage name constants used by LatencyTracker and pipeline instrumentation.
const (
	StageCapture   = "capture"
	StageDecode    = "decode"
	StageTranslate = "translate"
	StageEncode    = "encode"
	StageSend      = "send"
)

// ChunkLatency holds per-chunk timing data. It is a value struct (not a
// pointer) so that the sync.Pool can recycle it with minimal allocations.
// The backing array for StageTimings is reused across chunks via [:0].
type ChunkLatency struct {
	ChunkID      string
	StageTimings []StageTiming
	Total        time.Duration
}

// StageTiming records one pipeline stage's timing: the stage name, start
// and end timestamps, and the calculated duration.
type StageTiming struct {
	Stage    string
	StartAt  time.Time
	EndAt    time.Time
	Duration time.Duration
}

// LatencyTracker records per-chunk timing for one pipeline half. Each half
// (A→B, B→A) gets its own tracker — there is no shared mutable state.
//
// The tracker uses a sync.Pool to recycle ChunkLatency values, reusing the
// backing slice for StageTimings and keeping allocations below 3 per chunk
// after warmup.
type LatencyTracker struct {
	pool    *sync.Pool    // reuses ChunkLatency + StageTimings backing slice
	current *ChunkLatency // in-flight chunk; nil between Emit and next Reset
	chunkID int64         // auto-incrementing per half (1-based)
	mu      sync.Mutex    // guards current and chunkID
}

// NewLatencyTracker creates a tracker with a sync.Pool that pre-allocates
// ChunkLatency values with a 5-capacity StageTimings slice — one slot per
// pipeline stage.
func NewLatencyTracker() *LatencyTracker {
	return &LatencyTracker{
		pool: &sync.Pool{
			New: func() any {
				return &ChunkLatency{
					StageTimings: make([]StageTiming, 0, 5),
				}
			},
		},
	}
}

// StartStage records the start time for the given stage. The stage name
// MUST be one of the package constants (StageCapture, StageDecode, etc.).
//
// The tracker expects StartStage/EndStage pairs to be called in order. A
// second StartStage for the same stage before its EndStage appends a new
// entry — callers should avoid this.
func (lt *LatencyTracker) StartStage(stage string) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	if lt.current == nil {
		return
	}
	lt.current.StageTimings = append(lt.current.StageTimings, StageTiming{
		Stage:   stage,
		StartAt: time.Now(),
	})
}

// EndStage records the end time and calculates Duration for the most
// recent unclosed entry with the matching stage name. If no matching
// entry is found (e.g. called before StartStage), EndStage is a no-op.
func (lt *LatencyTracker) EndStage(stage string) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	if lt.current == nil {
		return
	}
	for i := len(lt.current.StageTimings) - 1; i >= 0; i-- {
		st := &lt.current.StageTimings[i]
		if st.Stage == stage && st.EndAt.IsZero() {
			now := time.Now()
			st.EndAt = now
			st.Duration = now.Sub(st.StartAt)
			return
		}
	}
}

// stageMsKey returns the "_ms" suffixed key for a standard stage name
// without allocating. For non-standard stage names it falls back to concat.
func stageMsKey(stage string) string {
	switch stage {
	case StageCapture:
		return "capture_ms"
	case StageDecode:
		return "decode_ms"
	case StageTranslate:
		return "translate_ms"
	case StageEncode:
		return "encode_ms"
	case StageSend:
		return "send_ms"
	default:
		return stage + "_ms"
	}
}

// Emit logs a structured chunk_latency event with all stage timings and
// total duration, then returns the ChunkLatency to the pool for reuse.
//
// Fields logged:
//
//	msg: "chunk_latency"
//	component: "pipeline"
//	room_id, half, chunk_id, status
//	total_ms
//	stages: { capture_ms, decode_ms, translate_ms, encode_ms, send_ms }
//
// After Emit, the tracker has no current chunk. Call Reset() before the
// next chunk.
//
// Allocations after warmup: 1 (ChunkID string) + possible slog internal.
// Stage key strings use compile-time constants (no concat).
func (lt *LatencyTracker) Emit(ctx context.Context, half, roomID, status string) {
	lt.mu.Lock()
	cl := lt.current
	lt.current = nil
	lt.mu.Unlock()

	if cl == nil {
		return
	}

	// Sum per-stage durations for total.
	var total time.Duration
	for _, st := range cl.StageTimings {
		total += st.Duration
	}
	cl.Total = total

	// Build stage attrs into a fixed-size array (max 5 stages).
	// This avoids heap allocation for the attr slice.
	n := len(cl.StageTimings)
	var stageArr [5]slog.Attr
	for i, st := range cl.StageTimings {
		stageArr[i] = slog.Int64(stageMsKey(st.Stage), st.Duration.Milliseconds())
	}

	// Build top-level attrs into a fixed-size array.
	var attrArr [7]slog.Attr
	attrArr[0] = slog.String("component", "pipeline")
	attrArr[1] = slog.String("room_id", roomID)
	attrArr[2] = slog.String("half", half)
	attrArr[3] = slog.String("chunk_id", cl.ChunkID)
	attrArr[4] = slog.String("status", status)
	attrArr[5] = slog.Int64("total_ms", total.Milliseconds())
	attrArr[6] = slog.Attr{
		Key:   "stages",
		Value: slog.GroupValue(stageArr[:n]...),
	}

	slog.LogAttrs(ctx, slog.LevelInfo, "chunk_latency", attrArr[:]...)

	// Return to pool — the next Reset() will Get() the same (or recycled) pointer.
	lt.pool.Put(cl)
}

// Reset prepares the tracker for a new chunk. It increments the internal
// chunk ID counter, acquires a ChunkLatency from the pool, sets its ID,
// and clears the StageTimings slice (keeping the backing array for reuse).
//
// Reset MUST be called before StartStage for each new chunk. It is safe
// to call without a preceding Emit (the previous chunk is discarded).
func (lt *LatencyTracker) Reset() {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.chunkID++
	cl := lt.pool.Get().(*ChunkLatency)
	cl.ChunkID = strconv.FormatInt(lt.chunkID, 10)
	cl.StageTimings = cl.StageTimings[:0]
	cl.Total = 0
	lt.current = cl
}
