package driven

import "context"

// PeerConnectionState represents the connection state of a WebRTC peer.
type PeerConnectionState int

const (
	// PeerStateNew means the peer connection has been created but ICE gathering has not started.
	PeerStateNew PeerConnectionState = iota
	// PeerStateConnecting means ICE candidates are being exchanged.
	PeerStateConnecting
	// PeerStateConnected means the peer connection is established and media can flow.
	PeerStateConnected
	// PeerStateDisconnected means the connection was interrupted but may recover.
	PeerStateDisconnected
	// PeerStateFailed means the connection failed and cannot recover.
	PeerStateFailed
	// PeerStateClosed means the connection was closed.
	PeerStateClosed
)

// WebRTCPeer defines the driven port to manage WebRTC peer connections.
// The primary implementation is the Pion adapter in internal/adapters/webrtc/.
type WebRTCPeer interface {
	// CreateSession sets up a new Pion PeerConnection for the given sessionID.
	// Configures STUN-only ICE servers. Returns an error if sessionID already exists.
	CreateSession(ctx context.Context, sessionID string) error

	// CloseSession closes the Pion peer connection and releases all resources.
	// Idempotent: calling it on a non-existent session does NOT return an error.
	CloseSession(ctx context.Context, sessionID string) error

	// HandleOffer sets the remote SDP description on the peer connection.
	// Must be called after CreateSession.
	HandleOffer(ctx context.Context, sessionID, sdp string) error

	// CreateAnswer generates a local SDP answer and sets it as the local description.
	// Must be called after HandleOffer. Returns the SDP answer string.
	CreateAnswer(ctx context.Context, sessionID string) (string, error)

	// AddICECandidate parses and adds an ICE candidate to the peer connection.
	// Candidates received before remote description is set should be buffered.
	AddICECandidate(ctx context.Context, sessionID, candidate string) error

	// OnICECandidate registers a callback that fires for each gathered local ICE candidate.
	// The callback is called once per candidate (trickle ICE) and stops after gathering completes.
	OnICECandidate(ctx context.Context, sessionID string, handler func(candidate string)) error

	// ConnectionState returns the current PeerConnectionState for the given sessionID.
	// Returns ErrSessionNotFound (from driving package) if sessionID does not exist.
	ConnectionState(ctx context.Context, sessionID string) (PeerConnectionState, error)
}
