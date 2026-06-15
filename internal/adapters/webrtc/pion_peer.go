// Package webrtc implements the WebRTCPeer driven port using Pion.
package webrtc

import (
	"context"
	"fmt"
	"sync"

	pionrtp "github.com/pion/rtp"
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

// audioHandler pairs an inbound audio handler with its cancellation context.
type audioHandler struct {
	fn  func(<-chan []byte)
	ctx context.Context
}

// PionPeer implements driven.WebRTCPeer using the Pion WebRTC library.
// Each session corresponds to one Pion PeerConnection keyed by sessionID.
type PionPeer struct {
	cfg              Config
	api              *pionwebrtc.API
	peers            map[string]*pionwebrtc.PeerConnection
	localTracks      map[string]*pionwebrtc.TrackLocalStaticRTP     // sessionID -> outbound track
	audioHandlers    map[string]audioHandler                        // sessionID -> registered inbound handler
	rawTracks        map[string]chan []byte                         // sessionID -> live audio channel (set when OnTrack fires)
	iceStateHandlers map[string]func(pionwebrtc.ICEConnectionState) // sessionID -> ICE state handler (for testing)
	mu               sync.RWMutex

	// OnICEFailed is called when the ICE connection for a session transitions to Failed.
	// Set this field before calling CreateSession for it to take effect.
	// If nil, ICE failures are silently ignored at this layer.
	OnICEFailed func(sessionID string)
}

// NewPionPeer creates a PionPeer with the given ICE server configuration.
func NewPionPeer(cfg Config) *PionPeer {
	m := &pionwebrtc.MediaEngine{}
	_ = m.RegisterDefaultCodecs()
	api := pionwebrtc.NewAPI(pionwebrtc.WithMediaEngine(m))
	return &PionPeer{
		cfg:              cfg,
		api:              api,
		peers:            make(map[string]*pionwebrtc.PeerConnection),
		localTracks:      make(map[string]*pionwebrtc.TrackLocalStaticRTP),
		audioHandlers:    make(map[string]audioHandler),
		rawTracks:        make(map[string]chan []byte),
		iceStateHandlers: make(map[string]func(pionwebrtc.ICEConnectionState)),
	}
}

// CreateSession sets up a new Pion PeerConnection configured for bidirectional audio (SendRecv).
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

	// Create outbound audio track for this session.
	track, err := pionwebrtc.NewTrackLocalStaticRTP(
		pionwebrtc.RTPCodecCapability{MimeType: pionwebrtc.MimeTypeOpus},
		"audio",
		fmt.Sprintf("talkgo-%s", sessionID),
	)
	if err != nil {
		_ = pc.Close()
		return fmt.Errorf("webrtc.CreateSession: create local track: %w", err)
	}

	if _, err := pc.AddTrack(track); err != nil {
		_ = pc.Close()
		return fmt.Errorf("webrtc.CreateSession: add track: %w", err)
	}

	_, err = pc.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeAudio, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionSendrecv,
	})
	if err != nil {
		_ = pc.Close()
		return fmt.Errorf("webrtc.CreateSession: adding audio transceiver: %w", err)
	}

	// OnTrack dispatches inbound audio to the registered handler for this session.
	// audioCh is stored in rawTracks so OnAudioTrack can fire the handler even if
	// it registers AFTER OnTrack fires (race condition fix).
	pc.OnTrack(func(remoteTrack *pionwebrtc.TrackRemote, _ *pionwebrtc.RTPReceiver) {
		audioCh := make(chan []byte, 32)

		p.mu.Lock()
		p.rawTracks[sessionID] = audioCh
		h, hasHandler := p.audioHandlers[sessionID]
		p.mu.Unlock()

		go func() {
			defer func() {
				close(audioCh)
				p.mu.Lock()
				delete(p.rawTracks, sessionID)
				p.mu.Unlock()
			}()
			for {
				pkt, _, readErr := remoteTrack.ReadRTP()
				if readErr != nil {
					return
				}
				if len(pkt.Payload) == 0 {
					continue
				}
				select {
				case audioCh <- append([]byte(nil), pkt.Payload...):
				default:
					// Drop frame if buffer full — backpressure handled upstream.
				}
			}
		}()

		if hasHandler {
			go h.fn(audioCh)
		}
	})

	// Register ICE connection state change handler.
	// When ICE transitions to Failed, call OnICEFailed (if set).
	iceHandler := func(state pionwebrtc.ICEConnectionState) {
		if state == pionwebrtc.ICEConnectionStateFailed {
			if p.OnICEFailed != nil {
				p.OnICEFailed(sessionID)
			}
		}
	}
	pc.OnICEConnectionStateChange(iceHandler)

	// Store handler reference for test introspection (already holding p.mu.Lock).
	p.iceStateHandlers[sessionID] = iceHandler

	p.peers[sessionID] = pc
	p.localTracks[sessionID] = track
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
	delete(p.localTracks, sessionID)
	delete(p.audioHandlers, sessionID)
	delete(p.rawTracks, sessionID)
	delete(p.iceStateHandlers, sessionID)

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

// CreateAnswer generates a local SDP answer, waits for ICE gathering to complete,
// and returns the final local description (with all candidates included).
// Waiting for gathering ensures relay candidates from TURN are present in the SDP,
// avoiding ICE failure when the server runs inside Docker (non-routable host candidates).
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

	// GatheringCompletePromise must be called BEFORE SetLocalDescription to avoid a race.
	gatherComplete := pionwebrtc.GatheringCompletePromise(pc)

	if err := pc.SetLocalDescription(answer); err != nil {
		return "", fmt.Errorf("webrtc.CreateAnswer: setting local description: %w", err)
	}

	// Block until ICE gathering finishes so TURN relay candidates are included in the SDP.
	<-gatherComplete

	return pc.LocalDescription().SDP, nil
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

// OnAudioTrack registers a handler that receives inbound Opus frames from the peer's media track.
// The handler is called with a read-only channel of RTP payloads. The channel is closed
// when the track ends or ctx is cancelled.
// If OnTrack already fired before this call, the handler is invoked immediately with the live channel.
func (p *PionPeer) OnAudioTrack(ctx context.Context, sessionID string, handler func(<-chan []byte)) error {
	p.mu.Lock()
	p.audioHandlers[sessionID] = audioHandler{fn: handler, ctx: ctx}
	ch, trackReady := p.rawTracks[sessionID]
	p.mu.Unlock()

	if trackReady {
		go handler(ch)
	}
	return nil
}

// SendAudio consumes Opus frames from audio and writes them to the peer's outbound track.
// Blocks until audio is closed or ctx is cancelled. Returns nil on clean shutdown.
func (p *PionPeer) SendAudio(ctx context.Context, sessionID string, audio <-chan []byte) error {
	p.mu.RLock()
	track, ok := p.localTracks[sessionID]
	p.mu.RUnlock()
	if !ok {
		return fmt.Errorf("webrtc.SendAudio: no track for session %s", sessionID)
	}

	var timestamp uint32
	var seqNum uint16
	packetsSent := 0
	for {
		select {
		case frame, open := <-audio:
			if !open {
				slog.Info("webrtc_send_audio_done", "session", sessionID, "packets_sent", packetsSent)
				return nil
			}
			if err := track.WriteRTP(&pionrtp.Packet{
				Header: pionrtp.Header{
					Version:        2,
					PayloadType:    111, // Opus
					SequenceNumber: seqNum,
					Timestamp:      timestamp,
					SSRC:           1,
				},
				Payload: frame,
			}); err != nil {
				return fmt.Errorf("webrtc.SendAudio: write: %w", err)
			}
			seqNum++
			packetsSent++
			if packetsSent == 1 {
				slog.Info("webrtc_rtp_first_packet", "session", sessionID, "seq", seqNum-1, "bytes", len(frame))
			}
			timestamp += 960 // 20ms at 48kHz
		case <-ctx.Done():
			return nil
		}
	}
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
