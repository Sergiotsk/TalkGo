package driven

import "context"

// AudioMixer defines the driven port for mixing audio streams (e.g. ambient + voice, or translation outputs).
type AudioMixer interface {
	// MixStreams mixes multiple audio channels into a single mono/stereo output.
	MixStreams(ctx context.Context, streams ...<-chan []byte) (<-chan []byte, error)
}
