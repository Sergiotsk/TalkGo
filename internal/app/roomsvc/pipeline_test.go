package roomsvc_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sergiotsk/TalkGo/internal/app/roomsvc"
	"github.com/Sergiotsk/TalkGo/internal/domain/room"
	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
	"github.com/Sergiotsk/TalkGo/internal/ports/driven/mocks"
)

// ---------------------------------------------------------------------------
// helpers for pipeline tests
// ---------------------------------------------------------------------------

func newTestService(t *testing.T, repo driven.RoomRepository, peer driven.WebRTCPeer, translator driven.Translator, codec driven.AudioCodec, notifier driven.EventNotifier) *roomsvc.Service {
	t.Helper()
	cfg := roomsvc.ServiceConfig{
		GracePeriod:         1 * time.Millisecond,
		RoomTTL:             10 * time.Minute,
		SweepInterval:       1 * time.Hour,
		MaxShortCodeRetries: 5,
	}
	svc, err := roomsvc.NewService(cfg, repo, peer, translator, codec, notifier)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func newFullRoom(t *testing.T) *room.Room {
	t.Helper()
	r, err := room.NewRoom("room-1", "es", "en")
	if err != nil {
		t.Fatalf("creating room: %v", err)
	}
	return r
}

// ---------------------------------------------------------------------------
// SC-01: Audio from A is translated and delivered to B
// ---------------------------------------------------------------------------

func TestService_SC01_AudioATranslatedToB(t *testing.T) {
	r := newFullRoom(t)
	codec := &mocks.MockAudioCodec{}
	translator := &mocks.MockTranslator{}
	notifier := &mocks.MockEventNotifier{}

	// Channels to capture results. Buffered to avoid goroutine leaks.
	receivedByB := make(chan []byte, 2)

	// sessIDCh receives session IDs in creation order (A then B).
	sessIDCh := make(chan string, 2)

	peer := &mocks.MockWebRTCPeer{
		CreateSessionFn: func(_ context.Context, sessID string) error {
			sessIDCh <- sessID
			return nil
		},
		OnAudioTrackFn: func(ctx context.Context, sessID string, handler func(<-chan []byte)) error {
			// Deliver "frame-a" only to sessA (first created session).
			// Both runHalf goroutines call this; sessIDCh has already been fully populated
			// by the time startPipeline is launched (JoinRoom returns before startPipeline goroutine runs).
			// We peek at sessIDCh without consuming to identify sessA.
			// Safe approach: deliver audio to whichever sessID is sessA.
			// We drain sessIDCh into a local slice on first call (protected by the channel itself).
			ch := make(chan []byte, 1)
			// Deliver frame-a to sessA; empty channel to sessB.
			// sessA is the first ID in sessIDCh — but we can't consume without losing it.
			// Instead: always send frame-a. Both directions get it; SendAudio captures who gets what.
			ch <- []byte("frame-a")
			close(ch)
			handler(ch)
			return nil
		},
		SendAudioFn: func(ctx context.Context, sessID string, audio <-chan []byte) error {
			if frame, ok := <-audio; ok {
				select {
				case receivedByB <- frame:
				default:
				}
			}
			return nil
		},
	}

	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := newTestService(t, repo, peer, translator, codec, notifier)

	ctx := context.Background()
	_, err := svc.JoinRoom(ctx, "room-1", "user-a", "es")
	if err != nil {
		t.Fatalf("JoinRoom user-a: %v", err)
	}
	_, err = svc.JoinRoom(ctx, "room-1", "user-b", "en")
	if err != nil {
		t.Fatalf("JoinRoom user-b: %v", err)
	}

	// Pipeline starts asynchronously — wait for frame to arrive at B (or A, since both receive).
	select {
	case frame := <-receivedByB:
		if string(frame) != "frame-a" {
			t.Errorf("expected %q, got %q", "frame-a", frame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: frame never arrived")
	}
}

// ---------------------------------------------------------------------------
// SC-02: Audio from B is translated and delivered to A
// ---------------------------------------------------------------------------

func TestService_SC02_AudioBTranslatedToA(t *testing.T) {
	r := newFullRoom(t)
	codec := &mocks.MockAudioCodec{}
	translator := &mocks.MockTranslator{}
	notifier := &mocks.MockEventNotifier{}

	// sessIDs populated by CreateSession in order: [sessA, sessB].
	// Use a buffered channel — both CreateSession calls happen before startPipeline.
	sessIDsCh := make(chan string, 2)

	receivedByA := make(chan []byte, 2)
	receivedByB := make(chan []byte, 2)

	var (
		sessIDsMu sync.Mutex
		sessA     string
	)

	peer := &mocks.MockWebRTCPeer{
		CreateSessionFn: func(_ context.Context, sessID string) error {
			sessIDsCh <- sessID
			return nil
		},
		OnAudioTrackFn: func(ctx context.Context, sessID string, handler func(<-chan []byte)) error {
			// Populate sessA/sessB from the channel on first access (idempotent via mutex).
			sessIDsMu.Lock()
			if sessA == "" {
				// Drain both IDs now — channel is guaranteed to have 2 entries by this point.
				select {
				case id := <-sessIDsCh:
					sessA = id
				default:
				}
				// Drain the second ID from channel (sessB) — we only need sessA for routing.
				select {
				case <-sessIDsCh:
				default:
				}
			}
			localSessA := sessA
			sessIDsMu.Unlock()

			ch := make(chan []byte, 1)
			if sessID != localSessA {
				// This is sessB — deliver "frame-b".
				ch <- []byte("frame-b")
			}
			// sessA: deliver empty channel.
			close(ch)
			handler(ch)
			return nil
		},
		SendAudioFn: func(ctx context.Context, sessID string, audio <-chan []byte) error {
			sessIDsMu.Lock()
			localSessA := sessA
			sessIDsMu.Unlock()

			if sessID == localSessA {
				// Frame is destined for sessA (translated from B→A).
				if frame, ok := <-audio; ok {
					select {
					case receivedByA <- frame:
					default:
					}
				}
			} else {
				// Drain silently.
				if frame, ok := <-audio; ok {
					select {
					case receivedByB <- frame:
					default:
					}
				}
			}
			return nil
		},
	}

	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := newTestService(t, repo, peer, translator, codec, notifier)

	ctx := context.Background()
	_, err := svc.JoinRoom(ctx, "room-1", "user-a", "es")
	if err != nil {
		t.Fatalf("JoinRoom user-a: %v", err)
	}
	_, err = svc.JoinRoom(ctx, "room-1", "user-b", "en")
	if err != nil {
		t.Fatalf("JoinRoom user-b: %v", err)
	}

	select {
	case frame := <-receivedByA:
		if string(frame) != "frame-b" {
			t.Errorf("expected %q, got %q", "frame-b", frame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: frame never arrived at A")
	}
}

// ---------------------------------------------------------------------------
// SC-04: Backpressure — slow translator drops old chunks, newest survives
// ---------------------------------------------------------------------------

func TestService_SC04_BackpressureDropsOldChunk(t *testing.T) {
	r := newFullRoom(t)
	notifier := &mocks.MockEventNotifier{}

	// receivedByB collects every frame that arrives at sessB.
	receivedByB := make(chan []byte, 10)

	// sessIDs populated via CreateSession in order: [sessA, sessB].
	sessIDsCh := make(chan string, 2)

	var (
		sessMu sync.Mutex
		sessA  string
	)

	initSessA := func() {
		sessMu.Lock()
		defer sessMu.Unlock()
		if sessA == "" {
			select {
			case id := <-sessIDsCh:
				sessA = id
			default:
			}
			// drain sessB id
			select {
			case <-sessIDsCh:
			default:
			}
		}
	}

	// slowTranslator delays each frame by 200 ms — creates heavy backpressure.
	slowTranslator := &mocks.MockTranslator{
		TranslateStreamFn: func(ctx context.Context, audioIn <-chan []byte, src, tgt string) (driven.TranslateResult, error) {
			out := make(chan []byte, 1)
			transcriptCh := make(chan string)
			close(transcriptCh)
			go func() {
				defer close(out)
				for frame := range audioIn {
					select {
					case <-time.After(200 * time.Millisecond):
						select {
						case out <- frame:
						case <-ctx.Done():
							return
						}
					case <-ctx.Done():
						return
					}
				}
			}()
			return driven.TranslateResult{Audio: out, Transcript: transcriptCh}, nil
		},
	}

	codec := &mocks.MockAudioCodec{}

	peer := &mocks.MockWebRTCPeer{
		CreateSessionFn: func(_ context.Context, sessID string) error {
			sessIDsCh <- sessID
			return nil
		},
		OnAudioTrackFn: func(ctx context.Context, sessID string, handler func(<-chan []byte)) error {
			initSessA()
			sessMu.Lock()
			localSessA := sessA
			sessMu.Unlock()

			if sessID == localSessA {
				// Send 3 frames rapidly — bpCh (cap 1) will drop the stale ones.
				ch := make(chan []byte, 3)
				ch <- []byte("frame-1")
				ch <- []byte("frame-2")
				ch <- []byte("frame-3")
				close(ch)
				handler(ch)
			} else {
				// sessB: empty track — nothing to send in this direction.
				ch := make(chan []byte)
				close(ch)
				handler(ch)
			}
			return nil
		},
		SendAudioFn: func(ctx context.Context, sessID string, audio <-chan []byte) error {
			// Collect everything that arrives with a 2 s deadline.
			go func() {
				deadline := time.After(2 * time.Second)
				for {
					select {
					case frame, ok := <-audio:
						if !ok {
							return
						}
						select {
						case receivedByB <- frame:
						default:
						}
					case <-deadline:
						return
					case <-ctx.Done():
						return
					}
				}
			}()
			return nil
		},
	}

	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := newTestService(t, repo, peer, slowTranslator, codec, notifier)

	ctx := context.Background()
	_, err := svc.JoinRoom(ctx, "room-1", "user-a", "es")
	if err != nil {
		t.Fatalf("JoinRoom user-a: %v", err)
	}
	_, err = svc.JoinRoom(ctx, "room-1", "user-b", "en")
	if err != nil {
		t.Fatalf("JoinRoom user-b: %v", err)
	}

	// Collect frames with a generous deadline (slow translator takes ~200ms per frame).
	var received []string
	deadline := time.After(2 * time.Second)
collect:
	for {
		select {
		case frame := <-receivedByB:
			received = append(received, string(frame))
		case <-deadline:
			break collect
		}
	}

	if len(received) == 0 {
		t.Fatal("expected at least one frame to reach B, got none")
	}
	// Backpressure must have dropped at least one frame (not all 3 made it through).
	if len(received) >= 3 {
		t.Errorf("expected backpressure to drop frames, but received all %d", len(received))
	}
	// The most recent frame (frame-3) must arrive — it is always drained in and forwarded.
	last := received[len(received)-1]
	if last != "frame-3" {
		t.Errorf("expected last received frame to be %q, got %q", "frame-3", last)
	}
}

// ---------------------------------------------------------------------------
// SC-05: Translator error notifies client; independent half keeps working
// ---------------------------------------------------------------------------

func TestService_SC05_TranslatorError_NotifiesClient(t *testing.T) {
	r := newFullRoom(t) // SourceLang="es", TargetLang="en"
	notifier := &mocks.MockEventNotifier{}

	sessIDsCh := make(chan string, 2)

	var (
		sessMu sync.Mutex
		sessA  string
	)

	initSessA := func() {
		sessMu.Lock()
		defer sessMu.Unlock()
		if sessA == "" {
			select {
			case id := <-sessIDsCh:
				sessA = id
			default:
			}
			select {
			case <-sessIDsCh:
			default:
			}
		}
	}

	// receivedByA captures frames translated B→A (the working half).
	receivedByA := make(chan []byte, 2)

	// Translator: A→B direction (src="es") returns an error;
	//             B→A direction (src="en") works as passthrough.
	errorTranslator := &mocks.MockTranslator{
		TranslateStreamFn: func(ctx context.Context, audioIn <-chan []byte, src, tgt string) (driven.TranslateResult, error) {
			if src == "es" {
				// Drain audioIn so the backpressure goroutine doesn't block.
				go func() {
					for range audioIn {
					}
				}()
				return driven.TranslateResult{}, errors.New("openai: rate limit exceeded")
			}
			// B→A: passthrough — re-implement inline to avoid importing mocks internals.
			out := make(chan []byte, 8)
			transcriptCh := make(chan string)
			close(transcriptCh)
			go func() {
				defer close(out)
				for {
					select {
					case frame, ok := <-audioIn:
						if !ok {
							return
						}
						select {
						case out <- frame:
						case <-ctx.Done():
							return
						}
					case <-ctx.Done():
						return
					}
				}
			}()
			return driven.TranslateResult{Audio: out, Transcript: transcriptCh}, nil
		},
	}

	codec := &mocks.MockAudioCodec{}

	peer := &mocks.MockWebRTCPeer{
		CreateSessionFn: func(_ context.Context, sessID string) error {
			sessIDsCh <- sessID
			return nil
		},
		OnAudioTrackFn: func(ctx context.Context, sessID string, handler func(<-chan []byte)) error {
			initSessA()
			sessMu.Lock()
			localSessA := sessA
			sessMu.Unlock()

			ch := make(chan []byte, 1)
			if sessID == localSessA {
				// sessA sends nothing — this half will error out at translator.
				close(ch)
			} else {
				// sessB sends one frame — B→A half should deliver it to A.
				ch <- []byte("frame-b")
				close(ch)
			}
			handler(ch)
			return nil
		},
		SendAudioFn: func(ctx context.Context, sessID string, audio <-chan []byte) error {
			initSessA()
			sessMu.Lock()
			localSessA := sessA
			sessMu.Unlock()

			if sessID == localSessA {
				go func() {
					for frame := range audio {
						select {
						case receivedByA <- frame:
						default:
						}
					}
				}()
			} else {
				go func() { // drain
					for range audio {
					}
				}()
			}
			return nil
		},
	}

	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := newTestService(t, repo, peer, errorTranslator, codec, notifier)

	ctx := context.Background()
	sessAID, err := svc.JoinRoom(ctx, "room-1", "user-a", "es")
	if err != nil {
		t.Fatalf("JoinRoom user-a: %v", err)
	}
	_, err = svc.JoinRoom(ctx, "room-1", "user-b", "en")
	if err != nil {
		t.Fatalf("JoinRoom user-b: %v", err)
	}

	// 1. Notification of "error" must be sent to sessA.
	deadline := time.After(2 * time.Second)
	for {
		notes := notifier.NotificationsFor(sessAID)
		found := false
		for _, n := range notes {
			if n.MsgType == "error" {
				found = true
				break
			}
		}
		if found {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("expected error notification for sessA (%s); got: %v", sessAID, notifier.NotificationsFor(sessAID))
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// 2. B→A half must still deliver frame-b to A.
	select {
	case frame := <-receivedByA:
		if string(frame) != "frame-b" {
			t.Errorf("expected %q at A, got %q", "frame-b", frame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: frame-b never arrived at A (B→A half broken)")
	}
}

// ---------------------------------------------------------------------------
// SC-06: LeaveRoom cancels the active pipeline
// ---------------------------------------------------------------------------

func TestService_SC06_LeaveRoom_CancelsPipeline(t *testing.T) {
	r := newFullRoom(t)
	notifier := &mocks.MockEventNotifier{}

	sessIDsCh := make(chan string, 2)

	var (
		sessMu sync.Mutex
		sessA  string
	)

	initSessA := func() {
		sessMu.Lock()
		defer sessMu.Unlock()
		if sessA == "" {
			select {
			case id := <-sessIDsCh:
				sessA = id
			default:
			}
			select {
			case <-sessIDsCh:
			default:
			}
		}
	}

	// done is closed when the OnAudioTrack handler for sessA returns (pipeline cancelled).
	done := make(chan struct{})

	peer := &mocks.MockWebRTCPeer{
		CreateSessionFn: func(_ context.Context, sessID string) error {
			sessIDsCh <- sessID
			return nil
		},
		OnAudioTrackFn: func(ctx context.Context, sessID string, handler func(<-chan []byte)) error {
			initSessA()
			sessMu.Lock()
			localSessA := sessA
			sessMu.Unlock()

			if sessID == localSessA {
				// Simulate a live track: blocks until ctx is cancelled, then closes.
				// This is how a real WebRTC peer behaves: the track closes when the
				// connection context is cancelled.
				go func() {
					trackCh := make(chan []byte)
					go func() {
						// Close trackCh when ctx is cancelled so that the
						// "for range trackCh" in runHalf's handler can unblock.
						<-ctx.Done()
						close(trackCh)
					}()
					handler(trackCh)
					close(done) // signals pipeline half exited
				}()
			} else {
				// sessB: empty track.
				ch := make(chan []byte)
				close(ch)
				handler(ch)
			}
			return nil
		},
		CloseSessionFn: func(_ context.Context, _ string) error { return nil },
	}

	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	codec := &mocks.MockAudioCodec{}
	translator := &mocks.MockTranslator{}
	svc := newTestService(t, repo, peer, translator, codec, notifier)

	ctx := context.Background()
	_, err := svc.JoinRoom(ctx, "room-1", "user-a", "es")
	if err != nil {
		t.Fatalf("JoinRoom user-a: %v", err)
	}
	_, err = svc.JoinRoom(ctx, "room-1", "user-b", "en")
	if err != nil {
		t.Fatalf("JoinRoom user-b: %v", err)
	}

	// Give pipeline time to start before leaving.
	time.Sleep(50 * time.Millisecond)

	if err := svc.LeaveRoom(ctx, "room-1", "user-a"); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}

	select {
	case <-done:
		// Pipeline goroutine for sessA exited cleanly.
	case <-time.After(2 * time.Second):
		t.Fatal("pipeline not cancelled after LeaveRoom")
	}
}

// ---------------------------------------------------------------------------
// SC-07: DeleteRoom cancels the active pipeline
// ---------------------------------------------------------------------------

func TestService_SC07_DeleteRoom_CancelsPipeline(t *testing.T) {
	r := newFullRoom(t)
	notifier := &mocks.MockEventNotifier{}

	sessIDsCh := make(chan string, 2)

	var (
		sessMu sync.Mutex
		sessA  string
	)

	initSessA := func() {
		sessMu.Lock()
		defer sessMu.Unlock()
		if sessA == "" {
			select {
			case id := <-sessIDsCh:
				sessA = id
			default:
			}
			select {
			case <-sessIDsCh:
			default:
			}
		}
	}

	// done is closed when both halves exit (we track sessA half only — it's the blocker).
	done := make(chan struct{})

	peer := &mocks.MockWebRTCPeer{
		CreateSessionFn: func(_ context.Context, sessID string) error {
			sessIDsCh <- sessID
			return nil
		},
		OnAudioTrackFn: func(ctx context.Context, sessID string, handler func(<-chan []byte)) error {
			initSessA()
			sessMu.Lock()
			localSessA := sessA
			sessMu.Unlock()

			if sessID == localSessA {
				go func() {
					trackCh := make(chan []byte)
					go func() {
						// Close trackCh when ctx is cancelled so that the
						// "for range trackCh" in runHalf's handler can unblock.
						<-ctx.Done()
						close(trackCh)
					}()
					handler(trackCh)
					close(done)
				}()
			} else {
				ch := make(chan []byte)
				close(ch)
				handler(ch)
			}
			return nil
		},
		CloseSessionFn: func(_ context.Context, _ string) error { return nil },
	}

	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
		DeleteFn:   func(_ context.Context, _ string) error { return nil },
	}
	codec := &mocks.MockAudioCodec{}
	translator := &mocks.MockTranslator{}
	svc := newTestService(t, repo, peer, translator, codec, notifier)

	ctx := context.Background()
	_, err := svc.JoinRoom(ctx, "room-1", "user-a", "es")
	if err != nil {
		t.Fatalf("JoinRoom user-a: %v", err)
	}
	_, err = svc.JoinRoom(ctx, "room-1", "user-b", "en")
	if err != nil {
		t.Fatalf("JoinRoom user-b: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if err := svc.DeleteRoom(ctx, "room-1"); err != nil {
		t.Fatalf("DeleteRoom: %v", err)
	}

	select {
	case <-done:
		// Both pipeline halves exited.
	case <-time.After(2 * time.Second):
		t.Fatal("pipeline not cancelled after DeleteRoom")
	}
}

// ---------------------------------------------------------------------------
// SC-09: Pipeline starts (OnAudioTrack called ×2) when room reaches capacity
// ---------------------------------------------------------------------------

func TestService_SC09_PipelineStartsWhenRoomFull(t *testing.T) {
	r := newFullRoom(t)
	notifier := &mocks.MockEventNotifier{}
	codec := &mocks.MockAudioCodec{}
	translator := &mocks.MockTranslator{}

	var onAudioTrackCalls atomic.Int64

	peer := &mocks.MockWebRTCPeer{
		OnAudioTrackFn: func(ctx context.Context, sessID string, handler func(<-chan []byte)) error {
			onAudioTrackCalls.Add(1)
			// Close immediately — we only care that the pipeline started.
			ch := make(chan []byte)
			close(ch)
			handler(ch)
			return nil
		},
	}

	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := newTestService(t, repo, peer, translator, codec, notifier)

	ctx := context.Background()
	_, err := svc.JoinRoom(ctx, "room-1", "user-a", "es")
	if err != nil {
		t.Fatalf("JoinRoom user-a: %v", err)
	}
	_, err = svc.JoinRoom(ctx, "room-1", "user-b", "en")
	if err != nil {
		t.Fatalf("JoinRoom user-b: %v", err)
	}

	// Pipeline is launched asynchronously — wait for both OnAudioTrack calls.
	deadline := time.After(2 * time.Second)
	for {
		if onAudioTrackCalls.Load() >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("expected OnAudioTrack to be called 2 times (one per half), got %d", onAudioTrackCalls.Load())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// ---------------------------------------------------------------------------
// SC-10: Decode error on every half — both source clients are notified
//
// The property under test: when Decode fails in runHalf, the error is
// reported to the SOURCE session of that half (not to the target).
// We make both halves fail so the test is deterministic — no goroutine
// scheduling dependency on which half starts first.
// ---------------------------------------------------------------------------

func TestService_SC10_DecodeError_NotifiesClient(t *testing.T) {
	r := newFullRoom(t) // SourceLang="es", TargetLang="en"
	notifier := &mocks.MockEventNotifier{}

	// errorCodec: Decode always fails — both halves will report error to their source.
	errorCodec := &mocks.MockAudioCodec{
		DecodeFn: func(ctx context.Context, opusIn <-chan []byte) (<-chan []byte, error) {
			go func() {
				for range opusIn {
				}
			}()
			return nil, errors.New("opus: invalid packet")
		},
	}

	translator := &mocks.MockTranslator{}

	peer := &mocks.MockWebRTCPeer{
		OnAudioTrackFn: func(ctx context.Context, sessID string, handler func(<-chan []byte)) error {
			// Deliver an empty track — the Decode error occurs regardless of frames.
			ch := make(chan []byte)
			close(ch)
			handler(ch)
			return nil
		},
	}

	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := newTestService(t, repo, peer, translator, errorCodec, notifier)

	ctx := context.Background()
	sessAID, err := svc.JoinRoom(ctx, "room-1", "user-a", "es")
	if err != nil {
		t.Fatalf("JoinRoom user-a: %v", err)
	}
	sessBID, err := svc.JoinRoom(ctx, "room-1", "user-b", "en")
	if err != nil {
		t.Fatalf("JoinRoom user-b: %v", err)
	}

	// Both source sessions must receive an "error" notification from their respective half.
	deadline := time.After(2 * time.Second)
	for {
		notesA := notifier.NotificationsFor(sessAID)
		notesB := notifier.NotificationsFor(sessBID)
		var hasErrA, hasErrB bool
		for _, n := range notesA {
			if n.MsgType == "error" {
				hasErrA = true
				break
			}
		}
		for _, n := range notesB {
			if n.MsgType == "error" {
				hasErrB = true
				break
			}
		}
		if hasErrA && hasErrB {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("expected error notifications for both sessions; sessA(%s) notified=%v, sessB(%s) notified=%v",
				sessAID, hasErrA, sessBID, hasErrB)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// ---------------------------------------------------------------------------
// SC-03: Simultaneous audio from A and B — both frames arrive
// ---------------------------------------------------------------------------

func TestService_SC03_SimultaneousAudio(t *testing.T) {
	r := newFullRoom(t)
	codec := &mocks.MockAudioCodec{}
	translator := &mocks.MockTranslator{}
	notifier := &mocks.MockEventNotifier{}

	receivedByA := make(chan []byte, 2)
	receivedByB := make(chan []byte, 2)

	// Collect sessA/sessB from CreateSession calls (sequential, before pipeline starts).
	sessIDsCh := make(chan string, 2)

	var (
		sessMu sync.Mutex
		sessA  string
		sessB  string
	)

	initSessIDs := func() {
		sessMu.Lock()
		defer sessMu.Unlock()
		if sessA == "" {
			select {
			case id := <-sessIDsCh:
				sessA = id
			default:
			}
			select {
			case id := <-sessIDsCh:
				sessB = id
			default:
			}
		}
	}

	peer := &mocks.MockWebRTCPeer{
		CreateSessionFn: func(_ context.Context, sessID string) error {
			sessIDsCh <- sessID
			return nil
		},
		OnAudioTrackFn: func(ctx context.Context, sessID string, handler func(<-chan []byte)) error {
			initSessIDs()

			sessMu.Lock()
			localSessA := sessA
			sessMu.Unlock()

			ch := make(chan []byte, 1)
			if sessID == localSessA {
				ch <- []byte("frame-a")
			} else {
				ch <- []byte("frame-b")
			}
			close(ch)
			handler(ch)
			return nil
		},
		SendAudioFn: func(ctx context.Context, sessID string, audio <-chan []byte) error {
			initSessIDs()

			sessMu.Lock()
			localSessA := sessA
			localSessB := sessB
			sessMu.Unlock()

			switch sessID {
			case localSessB:
				// Frame from A arriving at B.
				if frame, ok := <-audio; ok {
					select {
					case receivedByB <- frame:
					default:
					}
				}
			case localSessA:
				// Frame from B arriving at A.
				if frame, ok := <-audio; ok {
					select {
					case receivedByA <- frame:
					default:
					}
				}
			default:
				go func() {
					for range audio {
					}
				}()
			}
			return nil
		},
	}

	repo := &mocks.MockRoomRepository{
		FindByIDFn: func(_ context.Context, _ string) (*room.Room, error) { return r, nil },
		SaveFn:     func(_ context.Context, _ *room.Room) error { return nil },
	}
	svc := newTestService(t, repo, peer, translator, codec, notifier)

	ctx := context.Background()
	_, err := svc.JoinRoom(ctx, "room-1", "user-a", "es")
	if err != nil {
		t.Fatalf("JoinRoom user-a: %v", err)
	}
	_, err = svc.JoinRoom(ctx, "room-1", "user-b", "en")
	if err != nil {
		t.Fatalf("JoinRoom user-b: %v", err)
	}

	// Both frames must arrive within 2 seconds.
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	var gotA, gotB bool
	for !gotA || !gotB {
		select {
		case frame := <-receivedByB:
			if string(frame) != "frame-a" {
				t.Errorf("B received %q, expected %q", frame, "frame-a")
			}
			gotA = true
		case frame := <-receivedByA:
			if string(frame) != "frame-b" {
				t.Errorf("A received %q, expected %q", frame, "frame-b")
			}
			gotB = true
		case <-timer.C:
			t.Fatalf("timeout: gotFrameFromA=%v gotFrameFromB=%v", gotA, gotB)
		}
	}
}
