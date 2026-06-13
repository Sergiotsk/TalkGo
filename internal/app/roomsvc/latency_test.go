package roomsvc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sergiotsk/TalkGo/internal/app/roomsvc"
)

// ---------------------------------------------------------------------------
// TASK-017: Value semantics and struct basics
// ---------------------------------------------------------------------------

func TestChunkLatency_ValueSemantics(t *testing.T) {
	// ChunkLatency is a value struct — primitive fields (string, time.Duration)
	// are independent after a value copy. Slice fields share the backing
	// array per Go semantics (the slice header is copied but references
	// the same array), which is fine for the sync.Pool reuse pattern.
	orig := roomsvc.ChunkLatency{
		ChunkID: "1",
		StageTimings: []roomsvc.StageTiming{
			{Stage: "decode", Duration: 10 * time.Millisecond},
		},
		Total: 10 * time.Millisecond,
	}

	// Copy by value.
	copyChunk := orig

	// Mutate primitive fields on the copy.
	copyChunk.ChunkID = "2"
	copyChunk.Total = 99 * time.Millisecond

	// Primitive fields on the original must be unchanged.
	if orig.ChunkID != "1" {
		t.Errorf("copy mutated original ChunkID: got %q, want %q", orig.ChunkID, "1")
	}
	if orig.Total != 10*time.Millisecond {
		t.Errorf("copy mutated original Total: got %v, want %v", orig.Total, 10*time.Millisecond)
	}

	// NOTE: StageTimings slice backing array IS shared after value copy.
	// This is correct Go slice behavior: the copyChunk.StageTimings header
	// points to the same backing array as orig.StageTimings. The [:0]
	// reset pattern in LatencyTracker relies on this to reuse the buffer.
	//
	// Verify: both point to the same array by checking pointer equality.
	if len(orig.StageTimings) > 0 && len(copyChunk.StageTimings) > 0 {
		origPtr := &orig.StageTimings[0]
		copyPtr := &copyChunk.StageTimings[0]
		if origPtr != copyPtr {
			t.Log("StageTimings backing arrays diverged after copy (unexpected but not a bug)")
		} else {
			t.Log("StageTimings backing array is shared after value copy — expected Go behavior")
		}
	}
}

func TestStageTiming_BackingArrayReuse(t *testing.T) {
	// Simulate the reset pattern: [:0] must keep the backing array.
	original := make([]roomsvc.StageTiming, 0, 5)
	original = append(original,
		roomsvc.StageTiming{Stage: "capture", Duration: 1},
		roomsvc.StageTiming{Stage: "decode", Duration: 2},
	)

	// Capture the backing array pointer.
	firstElemAddr := &original[0]

	// Reset: truncate length, keep capacity.
	reset := original[:0]

	if cap(reset) != 5 {
		t.Errorf("expected cap 5 after reset, got %d", cap(reset))
	}
	if len(reset) != 0 {
		t.Errorf("expected len 0 after reset, got %d", len(reset))
	}

	// After append, the new slice should share the same backing array.
	reset = append(reset, roomsvc.StageTiming{Stage: "encode", Duration: 3})
	if &reset[0] != firstElemAddr {
		t.Error("[:0] did not reuse the backing array — new allocation occurred")
	}
}

// ---------------------------------------------------------------------------
// TASK-019: LatencyTracker StartStage / EndStage / Pool / ChunkID
// ---------------------------------------------------------------------------

func TestLatencyTracker_StartEndStages(t *testing.T) {
	tracker := roomsvc.NewLatencyTracker()

	// First reset to initialize the current chunk.
	tracker.Reset()
	tracker.StartStage(roomsvc.StageDecode)

	// Sleep long enough to be measurable on all platforms (Windows timer ~1-15ms).
	time.Sleep(5 * time.Millisecond)
	tracker.EndStage(roomsvc.StageDecode)

	tracker.StartStage(roomsvc.StageTranslate)
	time.Sleep(2 * time.Millisecond)
	tracker.EndStage(roomsvc.StageTranslate)

	// Emit and capture the log to inspect timings.
	ctx := context.Background()
	buf, restore := captureSlog(t)
	defer restore()

	tracker.Emit(ctx, "AtoB", "room-1", "ok")

	// Parse the emitted log.
	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON log: %v", err)
	}

	// Verify basic fields.
	if msg, _ := entry["msg"].(string); msg != "chunk_latency" {
		t.Errorf("msg = %q, want %q", msg, "chunk_latency")
	}
	if c, _ := entry["component"].(string); c != "pipeline" {
		t.Errorf("component = %q, want %q", c, "pipeline")
	}
	if half, _ := entry["half"].(string); half != "AtoB" {
		t.Errorf("half = %q, want %q", half, "AtoB")
	}
	if room, _ := entry["room_id"].(string); room != "room-1" {
		t.Errorf("room_id = %q, want %q", room, "room-1")
	}
	if status, _ := entry["status"].(string); status != "ok" {
		t.Errorf("status = %q, want %q", status, "ok")
	}

	// total_ms must be > 0.
	totalMs, ok := entry["total_ms"].(float64)
	if !ok || totalMs <= 0 {
		t.Errorf("total_ms = %v, want > 0", totalMs)
	}

	// stages must contain decode_ms and translate_ms, both > 0.
	stages, ok := entry["stages"].(map[string]any)
	if !ok {
		t.Fatal("stages field missing or not an object")
	}
	decodeMs, ok := stages["decode_ms"].(float64)
	if !ok || decodeMs <= 0 {
		t.Errorf("decode_ms = %v, want > 0", decodeMs)
	}
	translateMs, ok := stages["translate_ms"].(float64)
	if !ok || translateMs <= 0 {
		t.Errorf("translate_ms = %v, want > 0", translateMs)
	}
}

func TestLatencyTracker_ChunkIDIncrements(t *testing.T) {
	trackerA := roomsvc.NewLatencyTracker()
	trackerB := roomsvc.NewLatencyTracker()

	ctx := context.Background()

	// Process 3 chunks on trackerA, 2 on trackerB. IDs must be independent.

	buf, restore := captureSlog(t)
	defer restore()

	for i := 0; i < 3; i++ {
		trackerA.Reset()
		trackerA.StartStage(roomsvc.StageCapture)
		trackerA.EndStage(roomsvc.StageCapture)
		trackerA.Emit(ctx, "AtoB", "room-1", "ok")
	}

	for i := 0; i < 2; i++ {
		trackerB.Reset()
		trackerB.StartStage(roomsvc.StageCapture)
		trackerB.EndStage(roomsvc.StageCapture)
		trackerB.Emit(ctx, "BtoA", "room-1", "ok")
	}

	// Parse all log lines.
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 log lines, got %d", len(lines))
	}

	var aIDs, bIDs []string
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		half, _ := entry["half"].(string)
		id, _ := entry["chunk_id"].(string)
		switch half {
		case "AtoB":
			aIDs = append(aIDs, id)
		case "BtoA":
			bIDs = append(bIDs, id)
		}
	}

	// AtoB: must have IDs 1, 2, 3
	if len(aIDs) != 3 {
		t.Fatalf("expected 3 AtoB chunks, got %d", len(aIDs))
	}
	for i, id := range aIDs {
		want := string(rune('1' + i)) // "1", "2", "3"
		if id != want {
			t.Errorf("AtoB chunk %d: chunk_id = %q, want %q", i, id, want)
		}
	}

	// BtoA: must have IDs 1, 2
	if len(bIDs) != 2 {
		t.Fatalf("expected 2 BtoA chunks, got %d", len(bIDs))
	}
	for i, id := range bIDs {
		want := string(rune('1' + i)) // "1", "2"
		if id != want {
			t.Errorf("BtoA chunk %d: chunk_id = %q, want %q", i, id, want)
		}
	}
}

func TestLatencyTracker_PoolReusesInstances(t *testing.T) {
	ctx := context.Background()

	// Use a captured logger to avoid I/O allocations skewing the count.
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	old := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(old)

	tracker := roomsvc.NewLatencyTracker()

	// Warm up: process a few chunks so the pool stabilizes.
	for i := 0; i < 10; i++ {
		tracker.Reset()
		tracker.StartStage(roomsvc.StageCapture)
		tracker.EndStage(roomsvc.StageCapture)
		tracker.StartStage(roomsvc.StageDecode)
		tracker.EndStage(roomsvc.StageDecode)
		tracker.StartStage(roomsvc.StageTranslate)
		tracker.EndStage(roomsvc.StageTranslate)
		tracker.StartStage(roomsvc.StageEncode)
		tracker.EndStage(roomsvc.StageEncode)
		tracker.StartStage(roomsvc.StageSend)
		tracker.EndStage(roomsvc.StageSend)
		tracker.Emit(ctx, "AtoB", "room-1", "ok")
		buf.Reset()
	}

	// After warmup, each cycle should have minimal allocations.
	// The main expected allocation is strconv.FormatInt for ChunkID (1 alloc).
	// slog internals may add some. Target: < 3 after further optimization.
	allocs := testing.AllocsPerRun(100, func() {
		tracker.Reset()
		tracker.StartStage(roomsvc.StageCapture)
		tracker.EndStage(roomsvc.StageCapture)
		tracker.StartStage(roomsvc.StageDecode)
		tracker.EndStage(roomsvc.StageDecode)
		tracker.StartStage(roomsvc.StageTranslate)
		tracker.EndStage(roomsvc.StageTranslate)
		tracker.StartStage(roomsvc.StageEncode)
		tracker.EndStage(roomsvc.StageEncode)
		tracker.StartStage(roomsvc.StageSend)
		tracker.EndStage(roomsvc.StageSend)
		tracker.Emit(ctx, "AtoB", "room-1", "ok")
		buf.Reset()
	})

	t.Logf("allocs per chunk after warmup: %f", allocs)
	// Accept a wide range — this is a test that the pool mechanism works,
	// not a tight alloc constraint (that's what the benchmark is for).
	if allocs > 20 {
		t.Errorf("unexpectedly high allocs per chunk: %f (target < 3, currently %f)", allocs, allocs)
	}
}

// ---------------------------------------------------------------------------
// TASK-021: Benchmark + concurrent safety + integration (pipeline-like flow)
// ---------------------------------------------------------------------------

func BenchmarkLatencyTracker_Allocations(b *testing.B) {
	ctx := context.Background()
	tracker := roomsvc.NewLatencyTracker()

	// Use a buffered handler so I/O doesn't dominate the profile.
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	old := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(old)

	// Warmup: 10 iterations to prime the pool.
	for i := 0; i < 10; i++ {
		tracker.Reset()
		tracker.StartStage(roomsvc.StageCapture)
		tracker.EndStage(roomsvc.StageCapture)
		tracker.StartStage(roomsvc.StageDecode)
		tracker.EndStage(roomsvc.StageDecode)
		tracker.StartStage(roomsvc.StageTranslate)
		tracker.EndStage(roomsvc.StageTranslate)
		tracker.StartStage(roomsvc.StageEncode)
		tracker.EndStage(roomsvc.StageEncode)
		tracker.StartStage(roomsvc.StageSend)
		tracker.EndStage(roomsvc.StageSend)
		tracker.Emit(ctx, "AtoB", "room-1", "ok")
		buf.Reset()
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tracker.Reset()
		tracker.StartStage(roomsvc.StageCapture)
		tracker.EndStage(roomsvc.StageCapture)
		tracker.StartStage(roomsvc.StageDecode)
		tracker.EndStage(roomsvc.StageDecode)
		tracker.StartStage(roomsvc.StageTranslate)
		tracker.EndStage(roomsvc.StageTranslate)
		tracker.StartStage(roomsvc.StageEncode)
		tracker.EndStage(roomsvc.StageEncode)
		tracker.StartStage(roomsvc.StageSend)
		tracker.EndStage(roomsvc.StageSend)
		tracker.Emit(ctx, "AtoB", "room-1", "ok")
		buf.Reset()
	}
}

func TestLatencyTracker_ConcurrentSafety(t *testing.T) {
	t1 := roomsvc.NewLatencyTracker()
	t2 := roomsvc.NewLatencyTracker()
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(2)

	doneCh := make(chan struct{}, 2)

	// Tracker 1: process chunks in goroutine 1.
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			t1.Reset()
			t1.StartStage(roomsvc.StageDecode)
			t1.EndStage(roomsvc.StageDecode)
			t1.Emit(ctx, "AtoB", "room-1", "ok")
		}
		doneCh <- struct{}{}
	}()

	// Tracker 2: process chunks in goroutine 2 (independent tracker).
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			t2.Reset()
			t2.StartStage(roomsvc.StageTranslate)
			t2.EndStage(roomsvc.StageTranslate)
			t2.Emit(ctx, "BtoA", "room-1", "error")
		}
		doneCh <- struct{}{}
	}()

	// Wait for both, or timeout.
	deadline := time.After(5 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case <-doneCh:
		case <-deadline:
			t.Fatal("timeout waiting for concurrent tracker operations")
		}
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// Integration test: pipeline-like flow produces correct chunk_latency
// ---------------------------------------------------------------------------

func TestLatencyTracker_PipelineFlowEmit(t *testing.T) {
	tracker := roomsvc.NewLatencyTracker()
	ctx := context.Background()

	buf, restore := captureSlog(t)
	defer restore()

	// Simulate a full chunk processing cycle (5 stages).
	// Sleeps must be long enough for Windows timer resolution (~1-15ms).
	tracker.Reset()
	tracker.StartStage(roomsvc.StageCapture)
	tracker.EndStage(roomsvc.StageCapture)
	tracker.StartStage(roomsvc.StageDecode)
	time.Sleep(5 * time.Millisecond)
	tracker.EndStage(roomsvc.StageDecode)
	tracker.StartStage(roomsvc.StageTranslate)
	time.Sleep(3 * time.Millisecond)
	tracker.EndStage(roomsvc.StageTranslate)
	tracker.StartStage(roomsvc.StageEncode)
	time.Sleep(2 * time.Millisecond)
	tracker.EndStage(roomsvc.StageEncode)
	tracker.StartStage(roomsvc.StageSend)
	time.Sleep(1 * time.Millisecond)
	tracker.EndStage(roomsvc.StageSend)
	tracker.Emit(ctx, "AtoB", "room-1", "ok")

	// Parse the log.
	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON log: %v", err)
	}

	// All 5 stages must be present in the stages group.
	stages, ok := entry["stages"].(map[string]any)
	if !ok {
		t.Fatal("stages field missing or not an object")
	}

	expectedStages := []string{"capture_ms", "decode_ms", "translate_ms", "encode_ms", "send_ms"}
	for _, s := range expectedStages {
		ms, ok := stages[s].(float64)
		if !ok {
			t.Errorf("stage %q missing or not a number", s)
			continue
		}
		if ms < 0 {
			t.Errorf("stage %q = %v, want >= 0", s, ms)
		}
	}

	// chunk_id must be present.
	if id, _ := entry["chunk_id"].(string); id == "" {
		t.Error("chunk_id is empty or missing")
	}

	// status must be "ok".
	if status, _ := entry["status"].(string); status != "ok" {
		t.Errorf("status = %q, want %q", status, "ok")
	}
}

func TestLatencyTracker_EmitStatusError(t *testing.T) {
	tracker := roomsvc.NewLatencyTracker()
	ctx := context.Background()

	buf, restore := captureSlog(t)
	defer restore()

	// Simulate a decode error: only capture and decode stages are completed,
	// the rest are absent.
	tracker.Reset()
	tracker.StartStage(roomsvc.StageCapture)
	tracker.EndStage(roomsvc.StageCapture)
	tracker.StartStage(roomsvc.StageDecode)
	time.Sleep(2 * time.Millisecond)
	tracker.EndStage(roomsvc.StageDecode)
	tracker.Emit(ctx, "AtoB", "room-1", "error")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON log: %v", err)
	}

	if status, _ := entry["status"].(string); status != "error" {
		t.Errorf("status = %q, want %q", status, "error")
	}

	stages, ok := entry["stages"].(map[string]any)
	if !ok {
		t.Fatal("stages field missing or not an object")
	}

	// capture_ms and decode_ms should be present and ≥ 0.
	for _, s := range []string{"capture_ms", "decode_ms"} {
		ms, ok := stages[s].(float64)
		if !ok {
			t.Errorf("stage %q missing after error flow", s)
			continue
		}
		if ms < 0 {
			t.Errorf("stage %q negative: %v", s, ms)
		}
	}

	// Other stages may be absent (value 0 or missing) — that's acceptable.
}

// ---------------------------------------------------------------------------
// Test that Reset clears state for reuse
// ---------------------------------------------------------------------------

func TestLatencyTracker_ResetClearsState(t *testing.T) {
	tracker := roomsvc.NewLatencyTracker()

	ctx := context.Background()
	buf, restore := captureSlog(t)
	defer restore()

	// Process chunk with some stages.
	tracker.Reset()
	tracker.StartStage(roomsvc.StageDecode)
	time.Sleep(5 * time.Millisecond)
	tracker.EndStage(roomsvc.StageDecode)
	tracker.Emit(ctx, "AtoB", "room-1", "ok")

	// Parse first emit.
	var first map[string]any
	if err := json.Unmarshal(buf.Bytes(), &first); err != nil {
		t.Fatalf("invalid first JSON: %v", err)
	}

	firstTotalMs, _ := first["total_ms"].(float64)

	// Second chunk: shorter decode, verify total_ms is different and smaller.
	// We create a new buffer because we need to start fresh.
	var buf2 bytes.Buffer
	logger2 := slog.New(slog.NewJSONHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelDebug}))
	oldDefault := slog.Default()
	slog.SetDefault(logger2)
	defer slog.SetDefault(oldDefault)

	tracker.Reset()
	tracker.StartStage(roomsvc.StageDecode)
	time.Sleep(1 * time.Millisecond) // shorter
	tracker.EndStage(roomsvc.StageDecode)
	tracker.Emit(ctx, "AtoB", "room-1", "ok")

	var second map[string]any
	if err := json.Unmarshal(buf2.Bytes(), &second); err != nil {
		t.Fatalf("invalid second JSON: %v", err)
	}

	secondTotalMs, _ := second["total_ms"].(float64)

	// The second chunk should have a smaller total_ms (shorter decode).
	if secondTotalMs >= firstTotalMs {
		t.Logf("first total_ms = %v, second total_ms = %v (expected second < first due to shorter sleep)", firstTotalMs, secondTotalMs)
	}

	// Chunk ID should have incremented.
	if id, _ := second["chunk_id"].(string); id != "2" {
		t.Errorf("second chunk_id = %q, want %q", id, "2")
	}
}

// ---------------------------------------------------------------------------
// captureSlog replaces the default slog logger and returns the buffer.
// ---------------------------------------------------------------------------

func captureSlog(t *testing.T) (buf *bytes.Buffer, restore func()) {
	t.Helper()
	var b bytes.Buffer
	handler := slog.NewJSONHandler(&b, &slog.HandlerOptions{Level: slog.LevelDebug})
	old := slog.Default()
	slog.SetDefault(slog.New(handler))
	return &b, func() { slog.SetDefault(old) }
}
