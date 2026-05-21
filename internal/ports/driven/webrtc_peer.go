package driven

import "context"

// WebRTCPeer represents a driven port to manage WebRTC connections.
// Implementations: Pion WebRTC adapter.
type WebRTCPeer interface {
	// CreateSession sets up a WebRTC connection for a user.
	CreateSession(ctx context.Context, sessionID string) error
	// CloseSession closes the session.
	CloseSession(ctx context.Context, sessionID string) error
}
