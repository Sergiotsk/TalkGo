package main

import (
	"encoding/binary"
	"testing"
)

func TestLoadgen_AudioFrameSize(t *testing.T) {
	var phase float64
	frame := GenerateFrame(&phase)

	if len(frame) != FrameBytes {
		t.Errorf("frame length = %d bytes, want %d (samples=%d * 2 bytes/sample)",
			len(frame), FrameBytes, FrameSamples)
	}
}

func TestLoadgen_AudioFrameSampleCount(t *testing.T) {
	var phase float64
	frame := GenerateFrame(&phase)

	sampleCount := len(frame) / 2
	if sampleCount != FrameSamples {
		t.Errorf("sample count = %d, want %d", sampleCount, FrameSamples)
	}
}

func TestLoadgen_AudioFrameAmplitudeRange(t *testing.T) {
	var phase float64
	frame := GenerateFrame(&phase)

	for i := 0; i < len(frame); i += 2 {
		sample := int16(binary.LittleEndian.Uint16(frame[i:]))
		if sample < -32768 || sample > 32767 { //nolint:staticcheck // always false for int16 — documents that no clipping should occur
			t.Errorf("sample[%d] = %d, out of range [-32768, 32767]", i/2, sample)
		}
	}
}

func TestLoadgen_AudioFramePhasePersistence(t *testing.T) {
	var phase float64

	frame1 := GenerateFrame(&phase)
	frame2 := GenerateFrame(&phase)

	same := true
	for i := 0; i < len(frame1); i++ {
		if frame1[i] != frame2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("consecutive frames are identical, expected phase to advance")
	}
}

func TestLoadgen_AudioFrameSineShape(t *testing.T) {
	var phase float64
	frame := GenerateFrame(&phase)

	samples := make([]int16, FrameSamples)
	for i := 0; i < FrameSamples; i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(frame[i*2:]))
	}

	// A sine wave starting at phase=0 should begin near 0.
	if absInt(samples[0]) > 5000 {
		t.Errorf("expected first sample near 0, got %d", samples[0])
	}

	// The peak amplitude should be around ToneAmpl * 32767.
	var maxAbs int16
	for _, s := range samples {
		if absInt(s) > maxAbs {
			maxAbs = absInt(s)
		}
	}
	peak := float64(ToneAmpl) * 32767
	expectedMax := int16(peak)
	low := int16(peak * 0.9)
	high := int16(peak * 1.1)
	if maxAbs < low || maxAbs > high {
		t.Errorf("max amplitude = %d, want between %d and %d (expected ~%d)",
			maxAbs, low, high, expectedMax)
	}
}

func BenchmarkGenerateFrame(b *testing.B) {
	var phase float64
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = GenerateFrame(&phase)
	}
}

func absInt(x int16) int16 {
	if x < 0 {
		return -x
	}
	return x
}
