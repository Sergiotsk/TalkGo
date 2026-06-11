package driven

import "context"

// AudioCodec defines the driven port for encoding and decoding audio frames.
// The primary implementation converts between Opus and PCM16 at 24 kHz mono.
type AudioCodec interface {
	// Decode converts Opus frames from opusIn into PCM16 frames on the returned channel.
	// The output channel is closed when opusIn is closed or ctx is cancelled.
	Decode(ctx context.Context, opusIn <-chan []byte) (<-chan []byte, error)

	// Encode converts PCM16 frames from pcmIn into Opus frames on the returned channel.
	// The output channel is closed when pcmIn is closed or ctx is cancelled.
	Encode(ctx context.Context, pcmIn <-chan []byte) (<-chan []byte, error)
}
