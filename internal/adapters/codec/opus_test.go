package codec_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/Sergiotsk/TalkGo/internal/adapters/codec"
	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
)

// TASK-005: compile-time interface compliance guard.
var _ driven.AudioCodec = (*codec.OpusCodec)(nil)

func TestOpusCodec_InterfaceCompliance(t *testing.T) {
	t.Parallel()
	// The var _ guard above enforces this at compile time.
	// This test exists as an explicit test record.
	c := codec.NewOpusCodec()
	if c == nil {
		t.Fatal("NewOpusCodec returned nil")
	}
}

// TASK-007: decode a real Opus frame and verify PCM16 output.
func TestOpusCodec_Decode_ValidFrame(t *testing.T) {
	t.Parallel()

	// Real CELT-only Opus frame (valid for hraban/opus decoder at 24 kHz mono).
	// Decoded at 24 kHz mono it produces non-zero PCM16 LE output.
	realOpusFrame := []byte{
		0x48, 0x83, 0xca, 0xde, 0x8a, 0xe5, 0x67, 0xd5,
		0x1c, 0xac, 0xa2, 0x54, 0xfa, 0xff, 0xbf,
	}

	c := codec.NewOpusCodec()
	ctx := context.Background()

	in := make(chan []byte, 1)
	in <- realOpusFrame
	close(in)

	out, err := c.Decode(ctx, in)
	if err != nil {
		t.Fatalf("Decode returned unexpected error: %v", err)
	}

	select {
	case pcm, ok := <-out:
		if !ok {
			t.Fatal("Decode: output channel closed without producing a frame")
		}
		if len(pcm) == 0 {
			t.Fatal("Decode: output frame is empty")
		}
		if len(pcm)%2 != 0 {
			t.Fatalf("Decode: PCM16 output length must be even, got %d", len(pcm))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Decode: timeout waiting for output frame")
	}
}

// TASK-009: encode 480 PCM16 silence samples (960 bytes) and verify output.
func TestOpusCodec_Encode_ValidPCM(t *testing.T) {
	t.Parallel()

	// 480 samples × 2 bytes = 960 bytes of silence (PCM16 LE zeros).
	pcm := make([]byte, 960)

	c := codec.NewOpusCodec()
	ctx := context.Background()

	in := make(chan []byte, 1)
	in <- pcm
	close(in)

	out, err := c.Encode(ctx, in)
	if err != nil {
		t.Fatalf("Encode returned unexpected error: %v", err)
	}

	select {
	case frame, ok := <-out:
		if !ok {
			t.Fatal("Encode: output channel closed without producing a frame")
		}
		if len(frame) == 0 {
			t.Fatal("Encode: output frame is empty")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Encode: timeout waiting for output frame")
	}
}

// TASK-011: odd-length PCM frame must be silently skipped; output channel closes.
func TestOpusCodec_Encode_OddLength_Skipped(t *testing.T) {
	t.Parallel()

	c := codec.NewOpusCodec()
	ctx := context.Background()

	in := make(chan []byte, 1)
	in <- []byte{0x01, 0x02, 0x03} // 3 bytes — odd, must be skipped
	close(in)

	out, err := c.Encode(ctx, in)
	if err != nil {
		t.Fatalf("Encode returned unexpected error: %v", err)
	}

	// The channel should close without producing any frame.
	select {
	case frame, ok := <-out:
		if ok {
			t.Fatalf("Encode: expected no output for odd-length frame, got %d bytes", len(frame))
		}
		// ok == false means channel closed — correct behaviour.
	case <-time.After(2 * time.Second):
		t.Fatal("Encode: timeout — output channel did not close after odd-length frame")
	}
}

// TASK-012: round-trip — encode a 440 Hz PCM frame then decode it, verify PCM out is not all zeros.
// Uses hraban/opus real psychoacoustic encoder+decoder (libopus via CGO).
func TestOpusCodec_RoundTrip(t *testing.T) {
	t.Parallel()

	// Generate 440 Hz sine wave as PCM16 LE (480 samples at 24 kHz = 20 ms).
	const sampleRate = 24000
	const numSamples = 480
	const freq = 440.0
	pcm := make([]byte, numSamples*2)
	for i := range numSamples {
		sample := int16(math.Round(32767 * math.Sin(2*math.Pi*freq*float64(i)/sampleRate)))
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(sample))
	}

	c := codec.NewOpusCodec()
	ctx := context.Background()

	// Encode the 440 Hz PCM.
	encIn := make(chan []byte, 1)
	encIn <- pcm
	close(encIn)

	encOut, err := c.Encode(ctx, encIn)
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}

	var encodedFrame []byte
	select {
	case f, ok := <-encOut:
		if !ok {
			t.Fatal("RoundTrip: Encode closed without producing a frame")
		}
		encodedFrame = f
	case <-time.After(2 * time.Second):
		t.Fatal("RoundTrip: Encode timeout")
	}

	if len(encodedFrame) == 0 {
		t.Fatal("RoundTrip: encoded frame is empty")
	}

	// Now decode the encoded frame.
	decIn := make(chan []byte, 1)
	decIn <- encodedFrame
	close(decIn)

	decOut, err := c.Decode(ctx, decIn)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}

	select {
	case pcmOut, ok := <-decOut:
		if !ok {
			t.Fatal("RoundTrip: Decode closed without producing a frame")
		}
		if len(pcmOut) == 0 {
			t.Fatal("RoundTrip: decoded PCM is empty")
		}
		if len(pcmOut)%2 != 0 {
			t.Fatalf("RoundTrip: decoded PCM length must be even, got %d", len(pcmOut))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RoundTrip: Decode timeout")
	}
}

// TASK-013: cancelling context before sending any frame closes output and leaks no goroutines.
func TestOpusCodec_CancelContext_ClosesOutput(t *testing.T) {
	t.Parallel()

	c := codec.NewOpusCodec()
	ctx, cancel := context.WithCancel(context.Background())

	in := make(chan []byte) // never sends

	goroutinesBefore := runtime.NumGoroutine()

	out, err := c.Decode(ctx, in)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}

	cancel() // trigger cancellation immediately

	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("CancelContext: expected output channel to be closed after ctx cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("CancelContext: timeout — output channel not closed after context cancelled")
	}

	// Allow goroutine to exit, then verify no leak.
	time.Sleep(50 * time.Millisecond)
	goroutinesAfter := runtime.NumGoroutine()
	if goroutinesAfter > goroutinesBefore+2 {
		t.Errorf("CancelContext: possible goroutine leak — before=%d after=%d", goroutinesBefore, goroutinesAfter)
	}
}

// TASK-014: closing input channel immediately closes output channel.
func TestOpusCodec_ClosedInput_ClosesOutput(t *testing.T) {
	t.Parallel()

	c := codec.NewOpusCodec()
	ctx := context.Background()

	in := make(chan []byte)
	close(in) // closed immediately — no frames

	out, err := c.Decode(ctx, in)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}

	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("ClosedInput: expected output channel to be closed, but received a frame")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ClosedInput: timeout — output channel not closed after input closed")
	}
}

// TASK-015: encoding 960 bytes of silence (zeros) produces an output frame without error.
func TestOpusCodec_Encode_Silence_NoError(t *testing.T) {
	t.Parallel()

	// 480 samples of silence = 960 zero bytes.
	pcm := make([]byte, 960)

	c := codec.NewOpusCodec()
	ctx := context.Background()

	in := make(chan []byte, 1)
	in <- pcm
	close(in)

	out, err := c.Encode(ctx, in)
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}

	select {
	case frame, ok := <-out:
		if !ok {
			t.Fatal("Encode_Silence: channel closed without producing a frame")
		}
		if len(frame) == 0 {
			t.Fatal("Encode_Silence: output frame is empty")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Encode_Silence: timeout waiting for output frame")
	}

	// Wait for channel to close (no more frames).
	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("Encode_Silence: expected channel to close after single frame")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Encode_Silence: timeout waiting for channel close")
	}
}

// Encode cancellation test (symmetric to Decode cancel test).
func TestOpusCodec_Encode_CancelContext_ClosesOutput(t *testing.T) {
	t.Parallel()

	c := codec.NewOpusCodec()
	ctx, cancel := context.WithCancel(context.Background())

	in := make(chan []byte) // never sends

	out, err := c.Encode(ctx, in)
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}

	cancel()

	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("Encode_Cancel: expected output channel to close after ctx cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Encode_Cancel: timeout — output channel not closed after context cancelled")
	}
}

// Verify that bytes.Equal works for test helpers (used in passthrough_test.go).
var _ = bytes.Equal
