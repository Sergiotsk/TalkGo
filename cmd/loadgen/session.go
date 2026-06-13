package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// signalingMessage mirrors the server's SignalingMessage contract.
type signalingMessage struct {
	Type      string `json:"type"`
	RoomID    string `json:"room_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	SDP       string `json:"sdp,omitempty"`
	Candidate string `json:"candidate,omitempty"`
	Lang      string `json:"lang,omitempty"`
	Message   string `json:"message,omitempty"`
}

// runSession connects to the TalkGo server, performs signaling handshake,
// measures RTT via WebSocket ping/pong for the given duration, and returns
// a Report with all statistics.
func runSession(wsURL, roomID, lang string, duration time.Duration, profile string) (*Report, error) {
	report := &Report{
		Profile: profile,
		Errors:  make([]string, 0),
	}

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return report, fmt.Errorf("dial %s: %w", wsURL, err)
	}
	defer conn.Close() //nolint:errcheck // best-effort close

	// ---- Phase 1: Join ----
	userID := fmt.Sprintf("loadgen-%08x", rand.Uint32())
	joinMsg := signalingMessage{
		Type:   "join",
		RoomID: roomID,
		UserID: userID,
		Lang:   lang,
	}
	if err := conn.WriteJSON(joinMsg); err != nil {
		return report, fmt.Errorf("write join: %w", err)
	}

	var joined signalingMessage
	if err := conn.ReadJSON(&joined); err != nil {
		return report, fmt.Errorf("read joined: %w", err)
	}
	if joined.Type == "error" {
		return report, fmt.Errorf("join rejected: %s", joined.Message)
	}
	if joined.Type != "joined" || joined.SessionID == "" {
		return report, fmt.Errorf("unexpected join response: type=%s sessionID=%s", joined.Type, joined.SessionID)
	}

	sessionID := joined.SessionID

	// ---- Phase 2: Offer (dummy SDP - will likely fail, that's OK) ----
	offerMsg := signalingMessage{
		Type:      "offer",
		SessionID: sessionID,
		SDP:       "",
	}
	_ = conn.WriteJSON(offerMsg) // ignore write error — offer may fail

	// Read the offer response (expected to be an error for dummy SDP).
	var offerResp signalingMessage
	_ = conn.ReadJSON(&offerResp) // ignore read error — may be nothing

	// If we got a valid answer (unlikely without WebRTC), register a session.
	// If we got an error, that's expected — connection is still alive.
	_ = offerResp

	// ---- Phase 3: Ping loop (RTT measurement via WebSocket ping/pong) ----
	totalSent := 0
	var (
		mu         sync.Mutex
		rtts       []float64
		pongCount  int
		lastSendMu sync.Mutex
		lastSend   time.Time
	)

	// Set up the pong handler BEFORE the loop to avoid races.
	conn.SetPongHandler(func(appData string) error {
		now := time.Now()
		lastSendMu.Lock()
		sentAt := lastSend
		lastSendMu.Unlock()

		mu.Lock()
		if !sentAt.IsZero() {
			rtt := now.Sub(sentAt).Seconds() * 1000 // ms
			rtts = append(rtts, rtt)
		}
		pongCount++
		mu.Unlock()
		return nil
	})

	// Start a reader goroutine that reads and discards all data messages.
	// This is required for gorilla/websocket to process control frames
	// (including pong responses) even when no data messages are expected.
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	timer := time.NewTimer(duration)
	defer timer.Stop()

	pingTicker := time.NewTicker(20 * time.Millisecond)
	defer pingTicker.Stop()

loop:
	for {
		select {
		case <-timer.C:
			break loop
		case <-pingTicker.C:
			now := time.Now()
			lastSendMu.Lock()
			lastSend = now
			lastSendMu.Unlock()

			if err := conn.WriteControl(websocket.PingMessage, []byte("loadgen"), now.Add(time.Second)); err != nil {
				mu.Lock()
				report.Errors = append(report.Errors, fmt.Sprintf("ping write error: %v", err))
				mu.Unlock()
				break loop
			}
			totalSent++
		}
	}

	// ---- Phase 4: Leave ----
	leaveMsg := signalingMessage{
		Type:      "leave",
		SessionID: sessionID,
	}
	_ = conn.WriteJSON(leaveMsg)

	// Close the connection (this will cause the reader goroutine to exit).
	conn.Close() //nolint:errcheck // best-effort close
	<-readDone

	// Calculate duration in seconds (wall clock, not monotonic).
	// Since we use time.Timer, we approximate by the configured duration.
	durationSec := duration.Seconds()

	// ---- Phase 5: Compute report ----
	mu.Lock()
	report.AllRTTMs = rtts
	report.TotalMessages = totalSent
	report.DurationSec = durationSec

	if totalSent > 0 {
		report.PacketLossPct = float64(totalSent-pongCount) / float64(totalSent) * 100
		if report.PacketLossPct < 0 {
			report.PacketLossPct = 0
		}
	} else {
		report.PacketLossPct = 100
	}
	mu.Unlock()

	report.computeStats()
	report.computeStatus()

	return report, nil
}
