package main

import (
	"encoding/binary"
	"math"
)

// Audio constants.
const (
	SampleRate   = 24000                           // Hz
	FrameSizeMs  = 20                              // milliseconds per frame
	FrameSamples = SampleRate * FrameSizeMs / 1000 // 480 samples
	FrameBytes   = FrameSamples * 2                // 960 bytes (16-bit mono)
	ToneFreq     = 440.0                           // A4 frequency in Hz
	ToneAmpl     = 0.8                             // amplitude factor (80% of max)
)

// GenerateFrame produces a single audio frame of synthetic PCM 16-bit mono
// signed little-endian audio data (a 440Hz sine tone at 24kHz sample rate).
//
// The frame duration is 20ms, yielding 480 samples and 960 bytes.
// The amplitude is clamped to [-32768, 32767] — no clipping occurs with the
// default ToneAmpl of 0.8.
func GenerateFrame(phase *float64) []byte {
	frame := make([]byte, FrameBytes)
	for i := 0; i < FrameSamples; i++ {
		t := *phase + float64(i)/SampleRate
		sample := int16(math.Sin(2*math.Pi*ToneFreq*t) * ToneAmpl * 32767)
		binary.LittleEndian.PutUint16(frame[i*2:], uint16(sample))
	}
	*phase += float64(FrameSamples) / SampleRate
	return frame
}
