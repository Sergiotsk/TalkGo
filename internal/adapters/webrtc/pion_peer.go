// Package webrtc implements the WebRTCPeer driven port using Pion.
package webrtc

import (
	"context"
	"fmt"
	"sync"

	pionwebrtc "github.com/pion/webrtc/v3"

	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
	"github.com/Sergiotsk/TalkGo/internal/ports/driving"
)

// Config holds the ICE server configuration for Pion.
type Config struct {
	ICEServers []pionwebrtc.ICEServer
}

// DefaultConfig returns a STUN-only configuration using Google's public STUN server.
func DefaultConfig() Config {
	return Config{
		ICEServers: []pionwebrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}
}

// PionPeer implements driven.WebRTCPeer using the Pion WebRTC library.
// Each session corresponds to one Pion PeerConnection keyed by sessionID.
type PionPeer struct {
	cfg   Config
	api   *pionwebrtc.API
	peers map[string]*pionwebrtc.PeerConnection
	mu    sync.RWMutex
}

// NewPionPeer creates a PionPeer with the given ICE server configuration.
func NewPionPeer(cfg Config) *PionPeer {
	m := &pionwebrtc.MediaEngine{}
	_ = m.RegisterDefaultCodecs()
	api := pionwebrtc.NewAPI(pionwebrtc.WithMediaEngine(m))
	return &PionPeer{
		cfg:   cfg,
		api:   api,
		peers: make(map[string]*pionwebrtc.PeerConnection),
	}
}

// CreateSession sets up a new Pion PeerConnection configured for audio receive-only.
// Returns an error if a session with sessionID already exists.
func (p *PionPeer) CreateSession(_ context.Context, sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.peers[sessionID]; ok {
		return fmt.Errorf("webrtc.CreateSession: session %q already exists", sessionID)
	}

	pc, err := p.api.NewPeerConnection(pionwebrtc.Configuration{
		ICEServers: p.cfg.ICEServers,
	})
	if err != nil {
		return fmt.Errorf("webrtc.CreateSession: %w", err)
	}

	_, err = pc.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeAudio, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		_ = pc.Close()
		return fmt.Errorf("webrtc.CreateSession: adding audio transceiver: %w", err)
	}

	// Drain incoming RTP packets to prevent blocking.
	pc.OnTrack(func(track *pionwebrtc.TrackRemote, _ *pionwebrtc.RTPReceiver) {
		buf := make([]byte, 1500)
		for {
			if _, _, err := track.Read(buf); err != nil {
				return
			}
		}
	})

	p.peers[sessionID] = pc
	return nil
}

// CloseSession closes the Pion peer connection and releases resources.
// Idempotent: returns nil if sessionID does not exist.
func (p *PionPeer) CloseSession(_ context.Context, sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	pc, ok := p.peers[sessionID]
	if !ok {
		return nil
	}
	delete(p.peers, sessionID)
	if err := pc.Close(); err != nil {
		return fmt.Errorf("webrtc.CloseSession: %w", err)
	}
	return nil
}

// HandleOffer sets the remote SDP description on the peer connection.
func (p *PionPeer) HandleOffer(_ context.Context, sessionID, sdp string) error {
	p.mu.RLock()
	pc, ok := p.peers[sessionID]
	p.mu.RUnlock()
	if !ok {
		return fmt.Errorf("webrtc.HandleOffer: %w", driving.ErrSessionNotFound)
	}

	if err := pc.SetRemoteDescription(pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeOffer,
		SDP:  sdp,
	}); err != nil {
		return fmt.Errorf("webrtc.HandleOffer: %w", err)
	}
	return nil
}

// CreateAnswer generates a local SDP answer and sets it as the local description.
func (p *PionPeer) CreateAnswer(_ context.Context, sessionID string) (string, error) {
	p.mu.RLock()
	pc, ok := p.peers[sessionID]
	p.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("webrtc.CreateAnswer: %w", driving.ErrSessionNotFound)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return "", fmt.Errorf("webrtc.CreateAnswer: %w", err)
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		return "", fmt.Errorf("webrtc.CreateAnswer: setting local description: %w", err)
	}
	return answer.SDP, nil
}

// AddICECandidate adds an ICE candidate to the peer connection.
func (p *PionPeer) AddICECandidate(_ context.Context, sessionID, candidate string) error {
	p.mu.RLock()
	pc, ok := p.peers[sessionID]
	p.mu.RUnlock()
	if !ok {
		return fmt.Errorf("webrtc.AddICECandidate: %w", driving.ErrSessionNotFound)
	}

	if err := pc.AddICECandidate(pionwebrtc.ICECandidateInit{Candidate: candidate}); err != nil {
		return fmt.Errorf("webrtc.AddICECandidate: %w", err)
	}
	return nil
}

// OnICECandidate registers a callback for each locally gathered ICE candidate.
func (p *PionPeer) OnICECandidate(_ context.Context, sessionID string, handler func(string)) error {
	p.mu.RLock()
	pc, ok := p.peers[sessionID]
	p.mu.RUnlock()
	if !ok {
		return fmt.Errorf("webrtc.OnICECandidate: %w", driving.ErrSessionNotFound)
	}

	pc.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			return
		}
		handler(c.ToJSON().Candidate)
	})
	return nil
}

// ConnectionState returns the current PeerConnectionState for the given sessionID.
func (p *PionPeer) ConnectionState(_ context.Context, sessionID string) (driven.PeerConnectionState, error) {
	p.mu.RLock()
	pc, ok := p.peers[sessionID]
	p.mu.RUnlock()
	if !ok {
		return 0, fmt.Errorf("webrtc.ConnectionState: %w", driving.ErrSessionNotFound)
	}

	return pionStateToDriven(pc.ConnectionState()), nil
}

func pionStateToDriven(s pionwebrtc.PeerConnectionState) driven.PeerConnectionState {
	switch s {
	case pionwebrtc.PeerConnectionStateNew:
		return driven.PeerStateNew
	case pionwebrtc.PeerConnectionStateConnecting:
		return driven.PeerStateConnecting
	case pionwebrtc.PeerConnectionStateConnected:
		return driven.PeerStateConnected
	case pionwebrtc.PeerConnectionStateDisconnected:
		return driven.PeerStateDisconnected
	case pionwebrtc.PeerConnectionStateFailed:
		return driven.PeerStateFailed
	case pionwebrtc.PeerConnectionStateClosed:
		return driven.PeerStateClosed
	default:
		return driven.PeerStateNew
	}
}
