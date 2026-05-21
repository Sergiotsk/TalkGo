package driving

import "context"

// SignalingHandler defines the driving port to handle WebRTC signaling exchange.
type SignalingHandler interface {
	// HandleSignaling processes incoming WebRTC SDP offer/answer/ICE candidates.
	HandleSignaling(ctx context.Context, sessionID string, message []byte) ([]byte, error)
}
