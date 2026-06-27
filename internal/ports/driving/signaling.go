package driving

import (
	"context"
	"errors"
)

// ErrUnknownMessageType is returned when a SignalingMessage has an unrecognized Type.
var ErrUnknownMessageType = errors.New("unknown signaling message type")

// ErrSessionNotFound is returned when a session ID does not exist.
var ErrSessionNotFound = errors.New("session not found")

// SignalingMessage represents a typed WebRTC signaling message exchanged over WebSocket.
type SignalingMessage struct {
	// Type identifies the message kind.
	// Client→Server: "join" | "offer" | "ice-candidate" | "leave"
	// Server→Client: "joined" | "answer" | "ice-candidate" | "error" | "peer-left"
	Type      string `json:"type"`
	RoomID    string `json:"room_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	SDP       string `json:"sdp,omitempty"`
	Candidate string `json:"candidate,omitempty"`
	Message   string `json:"message,omitempty"`
	Lang      string `json:"lang,omitempty"`   // participant language on join
	Name      string `json:"name,omitempty"`   // display name sent on join; peer name relayed in joined/peer-joined
	Reason    string `json:"reason,omitempty"` // error reason for pipeline errors
}

// SignalingHandler defines the driving port for WebRTC signaling dispatch.
type SignalingHandler interface {
	// HandleSignaling processes an inbound signaling message and returns the response message.
	// Returns ErrUnknownMessageType for unrecognized message types.
	HandleSignaling(ctx context.Context, msg SignalingMessage) (SignalingMessage, error)

	// OnDisconnect is called by the Hub when a WebSocket client disconnects.
	// sessionID identifies the session that was lost (may be empty if join never completed).
	// Implementations should start a grace-period timer and notify peers.
	// Returns nil if sessionID is empty or not found (no-op).
	OnDisconnect(ctx context.Context, sessionID string) error
}
