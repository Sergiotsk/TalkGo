package webrtc

import (
	"strings"

	pionwebrtc "github.com/pion/webrtc/v3"
)

const stunURL = "stun:stun.l.google.com:19302"

// BuildICEConfig constructs a Config with ICE servers for Pion.
//
// When turnURLs is empty, the returned Config contains only the Google public STUN
// server (STUN-only mode, REQ-NET-03).
//
// When turnURLs is non-empty it must be a comma-separated list of TURN URLs
// (e.g. "turn:a:3478,turn:b:3478"). The returned Config contains two ICEServers:
// the STUN entry first, then a single TURN ICEServer whose URLs slice holds all
// parsed entries and whose Username / Credential fields are set to turnUser / turnPass
// (REQ-NET-01, REQ-NET-02).
func BuildICEConfig(turnURLs, turnUser, turnPass string) Config {
	servers := []pionwebrtc.ICEServer{
		{URLs: []string{stunURL}},
	}

	if turnURLs != "" {
		urls := strings.Split(turnURLs, ",")
		servers = append(servers, pionwebrtc.ICEServer{
			URLs:       urls,
			Username:   turnUser,
			Credential: turnPass,
		})
	}

	return Config{ICEServers: servers}
}
