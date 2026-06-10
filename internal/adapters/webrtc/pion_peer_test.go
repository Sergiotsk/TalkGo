package webrtc_test

import (
	"context"
	"testing"

	webrtcadapter "github.com/Sergiotsk/TalkGo/internal/adapters/webrtc"
)

func TestPionPeer_CreateAndCloseSession(t *testing.T) {
	peer := webrtcadapter.NewPionPeer(webrtcadapter.DefaultConfig())

	if err := peer.CreateSession(context.Background(), "sess-1"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Duplicate session must return error
	if err := peer.CreateSession(context.Background(), "sess-1"); err == nil {
		t.Error("expected error for duplicate sessionID")
	}

	// Close must succeed
	if err := peer.CloseSession(context.Background(), "sess-1"); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	// Close again must be idempotent
	if err := peer.CloseSession(context.Background(), "sess-1"); err != nil {
		t.Errorf("CloseSession idempotent: %v", err)
	}
}

func TestPionPeer_ConnectionState_UnknownSession(t *testing.T) {
	peer := webrtcadapter.NewPionPeer(webrtcadapter.DefaultConfig())

	_, err := peer.ConnectionState(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for unknown sessionID")
	}
}

func TestPionPeer_DefaultConfig(t *testing.T) {
	cfg := webrtcadapter.DefaultConfig()

	if len(cfg.ICEServers) == 0 {
		t.Error("expected at least one ICE server in DefaultConfig")
	}
	if len(cfg.ICEServers[0].URLs) == 0 {
		t.Error("expected STUN URL in first ICE server")
	}
}
