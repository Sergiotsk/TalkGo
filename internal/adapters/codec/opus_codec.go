// Package codec provides audio codec adapters for the TalkGo pipeline.
//
// Production note: this package currently ships a PassthroughCodec that
// forwards frames unchanged (no Opus ↔ PCM16 conversion). This is intentional
// for Windows development environments where CGO and libopus are unavailable.
//
// To replace with a real codec, implement the same driven.AudioCodec interface
// using a CGO-free library such as github.com/pion/opus (pure Go, no CGO) or
// gopkg.in/hraban/opus.v2 (requires CGO + libopus).
package codec

import (
	"context"

	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
)

// Interface guard — fails to compile if PassthroughCodec no longer satisfies AudioCodec.
var _ driven.AudioCodec = (*PassthroughCodec)(nil)

// PassthroughCodec implements driven.AudioCodec as a no-op passthrough.
//
// Frames are forwarded from the input channel to the output channel without
// any format conversion. The pipeline remains fully operational for integration
// and smoke testing; only the Opus ↔ PCM16 24 kHz mono conversion is absent.
//
// Replace with a real codec implementation before going to production.
type PassthroughCodec struct{}

// NewPassthroughCodec returns a PassthroughCodec ready to use.
func NewPassthroughCodec() *PassthroughCodec { return &PassthroughCodec{} }

// Decode forwards Opus frames from opusIn to the output channel unchanged.
// The output channel is closed when opusIn is closed or ctx is cancelled.
func (c *PassthroughCodec) Decode(ctx context.Context, opusIn <-chan []byte) (<-chan []byte, error) {
	return passthrough(ctx, opusIn), nil
}

// Encode forwards PCM16 frames from pcmIn to the output channel unchanged.
// The output channel is closed when pcmIn is closed or ctx is cancelled.
func (c *PassthroughCodec) Encode(ctx context.Context, pcmIn <-chan []byte) (<-chan []byte, error) {
	return passthrough(ctx, pcmIn), nil
}

// passthrough copies frames from in to the returned channel, stopping when in
// is closed or ctx is cancelled. The returned channel is always closed on exit.
func passthrough(ctx context.Context, in <-chan []byte) <-chan []byte {
	out := make(chan []byte, 8)
	go func() {
		defer close(out)
		for {
			select {
			case frame, ok := <-in:
				if !ok {
					return
				}
				select {
				case out <- frame:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
