package mocks

import (
	"context"

	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
)

// MockWebRTCPeer is a test double for driven.WebRTCPeer.
// Configure behaviour by assigning the Fn fields before use.
type MockWebRTCPeer struct {
	CreateSessionFn   func(ctx context.Context, sessionID string) error
	CloseSessionFn    func(ctx context.Context, sessionID string) error
	HandleOfferFn     func(ctx context.Context, sessionID, sdp string) error
	CreateAnswerFn    func(ctx context.Context, sessionID string) (string, error)
	AddICECandidateFn func(ctx context.Context, sessionID, candidate string) error
	OnICECandidateFn  func(ctx context.Context, sessionID string, handler func(string)) error
	ConnectionStateFn func(ctx context.Context, sessionID string) (driven.PeerConnectionState, error)

	CreateSessionCalled   int
	CloseSessionCalled    int
	HandleOfferCalled     int
	CreateAnswerCalled    int
	AddICECandidateCalled int
	OnICECandidateCalled  int
	ConnectionStateCalled int
}

// CreateSession implements driven.WebRTCPeer.
func (m *MockWebRTCPeer) CreateSession(ctx context.Context, sessionID string) error {
	m.CreateSessionCalled++
	if m.CreateSessionFn != nil {
		return m.CreateSessionFn(ctx, sessionID)
	}
	return nil
}

// CloseSession implements driven.WebRTCPeer.
func (m *MockWebRTCPeer) CloseSession(ctx context.Context, sessionID string) error {
	m.CloseSessionCalled++
	if m.CloseSessionFn != nil {
		return m.CloseSessionFn(ctx, sessionID)
	}
	return nil
}

// HandleOffer implements driven.WebRTCPeer.
func (m *MockWebRTCPeer) HandleOffer(ctx context.Context, sessionID, sdp string) error {
	m.HandleOfferCalled++
	if m.HandleOfferFn != nil {
		return m.HandleOfferFn(ctx, sessionID, sdp)
	}
	return nil
}

// CreateAnswer implements driven.WebRTCPeer.
func (m *MockWebRTCPeer) CreateAnswer(ctx context.Context, sessionID string) (string, error) {
	m.CreateAnswerCalled++
	if m.CreateAnswerFn != nil {
		return m.CreateAnswerFn(ctx, sessionID)
	}
	return "mock-sdp-answer", nil
}

// AddICECandidate implements driven.WebRTCPeer.
func (m *MockWebRTCPeer) AddICECandidate(ctx context.Context, sessionID, candidate string) error {
	m.AddICECandidateCalled++
	if m.AddICECandidateFn != nil {
		return m.AddICECandidateFn(ctx, sessionID, candidate)
	}
	return nil
}

// OnICECandidate implements driven.WebRTCPeer.
func (m *MockWebRTCPeer) OnICECandidate(ctx context.Context, sessionID string, handler func(string)) error {
	m.OnICECandidateCalled++
	if m.OnICECandidateFn != nil {
		return m.OnICECandidateFn(ctx, sessionID, handler)
	}
	return nil
}

// ConnectionState implements driven.WebRTCPeer.
func (m *MockWebRTCPeer) ConnectionState(ctx context.Context, sessionID string) (driven.PeerConnectionState, error) {
	m.ConnectionStateCalled++
	if m.ConnectionStateFn != nil {
		return m.ConnectionStateFn(ctx, sessionID)
	}
	return driven.PeerStateNew, nil
}
