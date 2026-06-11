package mocks

import (
	"context"
	"sync"
	"sync/atomic"

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
	OnAudioTrackFn    func(ctx context.Context, sessionID string, handler func(<-chan []byte)) error
	SendAudioFn       func(ctx context.Context, sessionID string, audio <-chan []byte) error

	createSessionCalled   atomic.Int64
	closeSessionCalled    atomic.Int64
	handleOfferCalled     atomic.Int64
	createAnswerCalled    atomic.Int64
	addICECandidateCalled atomic.Int64
	onICECandidateCalled  atomic.Int64
	connectionStateCalled atomic.Int64
	onAudioTrackCalled    atomic.Int64
	sendAudioCalled       atomic.Int64

	lastSendAudioMu        sync.Mutex
	lastSendAudioSessionID string
}

// CreateSessionCalled returns the number of CreateSession calls.
func (m *MockWebRTCPeer) CreateSessionCalled() int { return int(m.createSessionCalled.Load()) }

// CloseSessionCalled returns the number of CloseSession calls.
func (m *MockWebRTCPeer) CloseSessionCalled() int { return int(m.closeSessionCalled.Load()) }

// HandleOfferCalled returns the number of HandleOffer calls.
func (m *MockWebRTCPeer) HandleOfferCalled() int { return int(m.handleOfferCalled.Load()) }

// CreateAnswerCalled returns the number of CreateAnswer calls.
func (m *MockWebRTCPeer) CreateAnswerCalled() int { return int(m.createAnswerCalled.Load()) }

// AddICECandidateCalled returns the number of AddICECandidate calls.
func (m *MockWebRTCPeer) AddICECandidateCalled() int { return int(m.addICECandidateCalled.Load()) }

// OnICECandidateCalled returns the number of OnICECandidate calls.
func (m *MockWebRTCPeer) OnICECandidateCalled() int { return int(m.onICECandidateCalled.Load()) }

// ConnectionStateCalled returns the number of ConnectionState calls.
func (m *MockWebRTCPeer) ConnectionStateCalled() int { return int(m.connectionStateCalled.Load()) }

// OnAudioTrackCalled returns the number of OnAudioTrack calls.
func (m *MockWebRTCPeer) OnAudioTrackCalled() int { return int(m.onAudioTrackCalled.Load()) }

// SendAudioCalled returns the number of SendAudio calls.
func (m *MockWebRTCPeer) SendAudioCalled() int { return int(m.sendAudioCalled.Load()) }

// LastSendAudioSessionID returns the last sessionID passed to SendAudio.
func (m *MockWebRTCPeer) LastSendAudioSessionID() string {
	m.lastSendAudioMu.Lock()
	defer m.lastSendAudioMu.Unlock()
	return m.lastSendAudioSessionID
}

// CreateSession implements driven.WebRTCPeer.
func (m *MockWebRTCPeer) CreateSession(ctx context.Context, sessionID string) error {
	m.createSessionCalled.Add(1)
	if m.CreateSessionFn != nil {
		return m.CreateSessionFn(ctx, sessionID)
	}
	return nil
}

// CloseSession implements driven.WebRTCPeer.
func (m *MockWebRTCPeer) CloseSession(ctx context.Context, sessionID string) error {
	m.closeSessionCalled.Add(1)
	if m.CloseSessionFn != nil {
		return m.CloseSessionFn(ctx, sessionID)
	}
	return nil
}

// HandleOffer implements driven.WebRTCPeer.
func (m *MockWebRTCPeer) HandleOffer(ctx context.Context, sessionID, sdp string) error {
	m.handleOfferCalled.Add(1)
	if m.HandleOfferFn != nil {
		return m.HandleOfferFn(ctx, sessionID, sdp)
	}
	return nil
}

// CreateAnswer implements driven.WebRTCPeer.
func (m *MockWebRTCPeer) CreateAnswer(ctx context.Context, sessionID string) (string, error) {
	m.createAnswerCalled.Add(1)
	if m.CreateAnswerFn != nil {
		return m.CreateAnswerFn(ctx, sessionID)
	}
	return "mock-sdp-answer", nil
}

// AddICECandidate implements driven.WebRTCPeer.
func (m *MockWebRTCPeer) AddICECandidate(ctx context.Context, sessionID, candidate string) error {
	m.addICECandidateCalled.Add(1)
	if m.AddICECandidateFn != nil {
		return m.AddICECandidateFn(ctx, sessionID, candidate)
	}
	return nil
}

// OnICECandidate implements driven.WebRTCPeer.
func (m *MockWebRTCPeer) OnICECandidate(ctx context.Context, sessionID string, handler func(string)) error {
	m.onICECandidateCalled.Add(1)
	if m.OnICECandidateFn != nil {
		return m.OnICECandidateFn(ctx, sessionID, handler)
	}
	return nil
}

// ConnectionState implements driven.WebRTCPeer.
func (m *MockWebRTCPeer) ConnectionState(ctx context.Context, sessionID string) (driven.PeerConnectionState, error) {
	m.connectionStateCalled.Add(1)
	if m.ConnectionStateFn != nil {
		return m.ConnectionStateFn(ctx, sessionID)
	}
	return driven.PeerStateNew, nil
}

// OnAudioTrack implements driven.WebRTCPeer.
func (m *MockWebRTCPeer) OnAudioTrack(ctx context.Context, sessionID string, handler func(<-chan []byte)) error {
	m.onAudioTrackCalled.Add(1)
	if m.OnAudioTrackFn != nil {
		return m.OnAudioTrackFn(ctx, sessionID, handler)
	}
	return nil
}

// SendAudio implements driven.WebRTCPeer.
// When SendAudioFn is nil, the audio channel is drained in a goroutine and nil is returned.
func (m *MockWebRTCPeer) SendAudio(ctx context.Context, sessionID string, audio <-chan []byte) error {
	m.sendAudioCalled.Add(1)
	m.lastSendAudioMu.Lock()
	m.lastSendAudioSessionID = sessionID
	m.lastSendAudioMu.Unlock()
	if m.SendAudioFn != nil {
		return m.SendAudioFn(ctx, sessionID, audio)
	}
	go func() {
		for {
			select {
			case _, ok := <-audio:
				if !ok {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}
