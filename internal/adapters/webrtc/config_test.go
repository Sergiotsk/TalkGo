package webrtc_test

import (
	"strings"
	"testing"

	webrtcadapter "github.com/Sergiotsk/TalkGo/internal/adapters/webrtc"
)

// TestBuildICEConfig_STUNOnly verifies that passing empty TURN parameters returns
// exactly one ICEServer with a STUN URL and no TURN entry (REQ-NET-03).
func TestBuildICEConfig_STUNOnly(t *testing.T) {
	cfg := webrtcadapter.BuildICEConfig("", "", "")

	if len(cfg.ICEServers) != 1 {
		t.Fatalf("expected 1 ICEServer, got %d", len(cfg.ICEServers))
	}

	server := cfg.ICEServers[0]
	if len(server.URLs) == 0 {
		t.Fatal("expected at least one URL in STUN-only config")
	}

	for _, u := range server.URLs {
		if strings.HasPrefix(u, "turn:") || strings.HasPrefix(u, "turns:") {
			t.Errorf("unexpected TURN URL in STUN-only config: %s", u)
		}
	}

	if !strings.HasPrefix(server.URLs[0], "stun:") {
		t.Errorf("expected STUN URL, got %s", server.URLs[0])
	}
}

// TestBuildICEConfig_WithTURN verifies that a non-empty turnURLs produces exactly
// 2 ICEServers (STUN + TURN) with the correct URL and credentials (REQ-NET-01, REQ-NET-02).
func TestBuildICEConfig_WithTURN(t *testing.T) {
	cfg := webrtcadapter.BuildICEConfig("turn:srv:3478", "user", "pass")

	if len(cfg.ICEServers) != 2 {
		t.Fatalf("expected 2 ICEServers (STUN + TURN), got %d", len(cfg.ICEServers))
	}

	// First server must be STUN.
	stunServer := cfg.ICEServers[0]
	if len(stunServer.URLs) == 0 || !strings.HasPrefix(stunServer.URLs[0], "stun:") {
		t.Errorf("first ICEServer should be STUN, got URLs: %v", stunServer.URLs)
	}

	// Second server must be TURN with the provided URL.
	turnServer := cfg.ICEServers[1]
	if len(turnServer.URLs) == 0 {
		t.Fatal("TURN ICEServer has no URLs")
	}
	if turnServer.URLs[0] != "turn:srv:3478" {
		t.Errorf("expected TURN URL 'turn:srv:3478', got %s", turnServer.URLs[0])
	}
}

// TestBuildICEConfig_MultipleURLs verifies that comma-separated turnURLs produces a
// TURN ICEServer with multiple entries in its URLs slice (REQ-NET-01, D-06).
func TestBuildICEConfig_MultipleURLs(t *testing.T) {
	cfg := webrtcadapter.BuildICEConfig("turn:a:3478,turn:b:3478", "user", "pass")

	if len(cfg.ICEServers) != 2 {
		t.Fatalf("expected 2 ICEServers, got %d", len(cfg.ICEServers))
	}

	turnServer := cfg.ICEServers[1]
	if len(turnServer.URLs) != 2 {
		t.Errorf("expected 2 URLs in TURN ICEServer, got %d: %v", len(turnServer.URLs), turnServer.URLs)
	}

	if turnServer.URLs[0] != "turn:a:3478" {
		t.Errorf("expected first TURN URL 'turn:a:3478', got %s", turnServer.URLs[0])
	}
	if turnServer.URLs[1] != "turn:b:3478" {
		t.Errorf("expected second TURN URL 'turn:b:3478', got %s", turnServer.URLs[1])
	}
}

// TestBuildICEConfig_Credentials verifies that Username and Credential on the TURN
// ICEServer match the turnUser and turnPass inputs (REQ-NET-01).
func TestBuildICEConfig_Credentials(t *testing.T) {
	const turnUser = "myuser"
	const turnPass = "mysecret"

	cfg := webrtcadapter.BuildICEConfig("turn:srv:3478", turnUser, turnPass)

	if len(cfg.ICEServers) < 2 {
		t.Fatalf("expected at least 2 ICEServers, got %d", len(cfg.ICEServers))
	}

	turnServer := cfg.ICEServers[1]

	if turnServer.Username != turnUser {
		t.Errorf("expected Username %q, got %q", turnUser, turnServer.Username)
	}

	cred, ok := turnServer.Credential.(string)
	if !ok {
		t.Fatalf("expected Credential to be a string, got %T", turnServer.Credential)
	}
	if cred != turnPass {
		t.Errorf("expected Credential %q, got %q", turnPass, cred)
	}
}
