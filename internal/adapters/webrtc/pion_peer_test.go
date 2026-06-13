package webrtc_test

import (
	"context"
	"sync"
	"testing"
	"time"

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

// ---------------------------------------------------------------------------
// TASK-035: ICE Failed state triggers OnICEFailed callback (REQ-UX-01)
// ---------------------------------------------------------------------------

func TestPionPeer_ICEFailed_CallsOnICEFailedCallback(t *testing.T) {
	const sessID = "sess-ice-fail"

	peer := webrtcadapter.NewPionPeer(webrtcadapter.DefaultConfig())

	var (
		mu              sync.Mutex
		callbackSessID  string
		callbackInvoked bool
	)

	// Set the OnICEFailed callback before creating the session.
	peer.OnICEFailed = func(sessionID string) {
		mu.Lock()
		defer mu.Unlock()
		callbackSessID = sessionID
		callbackInvoked = true
	}

	if err := peer.CreateSession(context.Background(), sessID); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Simulate ICEConnectionStateFailed via the test export helper.
	if !peer.TriggerICEFailedForSession(sessID) {
		t.Fatal("TriggerICEFailedForSession returned false — no handler registered for sessionID")
	}

	// Allow the callback goroutine to fire (it may be async in pion internals).
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	invoked := callbackInvoked
	gotSessID := callbackSessID
	mu.Unlock()

	if !invoked {
		t.Fatal("OnICEFailed callback was not invoked after ICEConnectionStateFailed")
	}
	if gotSessID != sessID {
		t.Errorf("OnICEFailed called with sessionID=%q, want %q", gotSessID, sessID)
	}
}
