// export_test.go exposes internal PionPeer state for white-box testing.
// This file is only compiled during tests (package webrtc, not webrtc_test).
package webrtc

import pionwebrtc "github.com/pion/webrtc/v3"

// TriggerICEFailedForSession directly calls the iceStateHandlers entry for
// sessionID with ICEConnectionStateFailed, simulating what pion would call
// when the ICE negotiation fails. Returns false if no handler is registered.
// For testing only.
func (p *PionPeer) TriggerICEFailedForSession(sessionID string) bool {
	p.mu.RLock()
	h, ok := p.iceStateHandlers[sessionID]
	p.mu.RUnlock()
	if !ok {
		return false
	}
	h(pionwebrtc.ICEConnectionStateFailed)
	return true
}
