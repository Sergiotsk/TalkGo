// Package codec provides audio codec adapters for the TalkGo pipeline.
//
// CGO_ENABLED=1 required. Dockerfile must install libopus-dev.
package codec

import (
	"context"
	"log/slog"

	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
	"gopkg.in/hraban/opus.v2"
)

// Compile-time guard — fails to compile if OpusCodec no longer satisfies AudioCodec.
var _ driven.AudioCodec = (*OpusCodec)(nil)

const (
	// opusSampleRate is the target PCM sample rate for the Opus codec (24 kHz).
	opusSampleRate = 24000
	// opusChannels is the number of audio channels (mono).
	opusChannels = 1
	// opusFrameSize is the number of PCM samples per 20 ms frame at 24 kHz.
	opusFrameSize = 480
	// opusMaxPacketSize is the maximum Opus packet size in bytes.
	opusMaxPacketSize = 4096
)

// OpusCodec implements driven.AudioCodec using gopkg.in/hraban/opus.v2, which
// wraps libopus via CGO. It encodes/decodes between PCM16 LE at 24 kHz mono
// and real Opus frames using psychoacoustic compression (AppVoIP).
type OpusCodec struct{}

// NewOpusCodec returns an OpusCodec ready to use.
func NewOpusCodec() *OpusCodec { return &OpusCodec{} }

// Decode converts Opus frames from opusIn into PCM16 LE frames on the returned
// channel. The output channel is closed when opusIn is closed or ctx is
// cancelled.
func (c *OpusCodec) Decode(ctx context.Context, opusIn <-chan []byte) (<-chan []byte, error) {
	out := make(chan []byte, 8)
	go func() {
		defer close(out)
		dec, err := opus.NewDecoder(opusSampleRate, opusChannels)
		if err != nil {
			slog.Error("opus: failed to create decoder", "err", err)
			return
		}
		// Pre-allocate PCM int16 output buffer: opusFrameSize samples.
		pcmBuf := make([]int16, opusFrameSize)
		for {
			select {
			case frame, ok := <-opusIn:
				if !ok {
					return
				}
				n, decErr := dec.Decode(frame, pcmBuf)
				if decErr != nil {
					slog.Warn("opus: decode error, dropping frame", "err", decErr)
					continue
				}
				// Marshal []int16 → []byte PCM16 LE.
				out16 := pcmBuf[:n]
				pcmBytes := make([]byte, n*2)
				for i, s := range out16 {
					pcmBytes[i*2] = byte(s)
					pcmBytes[i*2+1] = byte(s >> 8)
				}
				select {
				case out <- pcmBytes:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// Encode converts PCM16 LE frames from pcmIn into Opus frames on the returned
// channel. Incoming chunks may be any size; samples are accumulated until a
// complete opusFrameSize-sample frame is available. The output channel is
// closed when pcmIn is closed or ctx is cancelled.
func (c *OpusCodec) Encode(ctx context.Context, pcmIn <-chan []byte) (<-chan []byte, error) {
	out := make(chan []byte, 8)
	go func() {
		defer close(out)
		enc, err := opus.NewEncoder(opusSampleRate, opusChannels, opus.AppVoIP)
		if err != nil {
			slog.Error("opus: failed to create encoder", "err", err)
			return
		}
		encodedBuf := make([]byte, opusMaxPacketSize)
		// accumBuf holds int16 samples waiting to form a complete frame.
		accumBuf := make([]int16, 0, opusFrameSize*2)

		encode := func() bool {
			for len(accumBuf) >= opusFrameSize {
				frame := accumBuf[:opusFrameSize]
				n, encErr := enc.Encode(frame, encodedBuf)
				if encErr != nil {
					slog.Warn("opus: encode error, dropping frame", "err", encErr)
				} else {
					pkt := make([]byte, n)
					copy(pkt, encodedBuf[:n])
					select {
					case out <- pkt:
					case <-ctx.Done():
						return false
					}
				}
				accumBuf = accumBuf[opusFrameSize:]
			}
			return true
		}

		for {
			select {
			case pcm, ok := <-pcmIn:
				if !ok {
					return
				}
				if len(pcm)%2 != 0 {
					slog.Warn("opus: skipping odd-length PCM frame", "len", len(pcm))
					continue
				}
				numSamples := len(pcm) / 2
				for i := range numSamples {
					accumBuf = append(accumBuf, int16(pcm[i*2])|int16(pcm[i*2+1])<<8)
				}
				if !encode() {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
