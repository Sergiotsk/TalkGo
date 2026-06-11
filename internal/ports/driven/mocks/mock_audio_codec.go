package mocks

import (
	"context"
	"sync/atomic"

	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
)

var _ driven.AudioCodec = (*MockAudioCodec)(nil)

// MockAudioCodec is a test double for driven.AudioCodec.
// Configure behaviour by assigning the Fn fields before use.
// When a Fn is nil the mock falls back to a passthrough that copies
// frames from the input channel to the output channel unchanged.
type MockAudioCodec struct {
	DecodeFn func(ctx context.Context, opusIn <-chan []byte) (<-chan []byte, error)
	EncodeFn func(ctx context.Context, pcmIn <-chan []byte) (<-chan []byte, error)

	decodeCalled atomic.Int64
	encodeCalled atomic.Int64
}

// DecodeCalled returns the number of Decode calls.
func (m *MockAudioCodec) DecodeCalled() int { return int(m.decodeCalled.Load()) }

// EncodeCalled returns the number of Encode calls.
func (m *MockAudioCodec) EncodeCalled() int { return int(m.encodeCalled.Load()) }

// Decode implements driven.AudioCodec.
func (m *MockAudioCodec) Decode(ctx context.Context, opusIn <-chan []byte) (<-chan []byte, error) {
	m.decodeCalled.Add(1)
	if m.DecodeFn != nil {
		return m.DecodeFn(ctx, opusIn)
	}
	return passthrough(ctx, opusIn), nil
}

// Encode implements driven.AudioCodec.
func (m *MockAudioCodec) Encode(ctx context.Context, pcmIn <-chan []byte) (<-chan []byte, error) {
	m.encodeCalled.Add(1)
	if m.EncodeFn != nil {
		return m.EncodeFn(ctx, pcmIn)
	}
	return passthrough(ctx, pcmIn), nil
}

// passthrough copies frames from in to a new channel without modification.
// The output channel is closed when in is closed or ctx is cancelled.
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
