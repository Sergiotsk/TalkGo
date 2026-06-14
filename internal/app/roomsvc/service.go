// Package roomsvc implements the application layer that orchestrates domain
// logic and driven ports to fulfil the RoomManager and SignalingHandler contracts.
package roomsvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Sergiotsk/TalkGo/internal/domain/room"
	"github.com/Sergiotsk/TalkGo/internal/domain/session"
	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
	"github.com/Sergiotsk/TalkGo/internal/ports/driving"
)

// ServiceConfig holds tunable parameters for the Service.
// Use small values (1ms) in tests to keep them fast.
type ServiceConfig struct {
	// GracePeriod is the time a room stays alive after the last participant disconnects.
	GracePeriod time.Duration
	// RoomTTL is the maximum inactivity duration before a room is swept away.
	RoomTTL time.Duration
	// SweepInterval controls how often the expiration goroutine runs.
	SweepInterval time.Duration
	// MaxShortCodeRetries is the maximum number of collision retries when generating a ShortCode.
	MaxShortCodeRetries int
}

// DefaultServiceConfig returns production-safe defaults.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		GracePeriod:         30 * time.Second,
		RoomTTL:             10 * time.Minute,
		SweepInterval:       60 * time.Second,
		MaxShortCodeRetries: 5,
	}
}

// Service implements driving.RoomManager and driving.SignalingHandler.
type Service struct {
	cfg        ServiceConfig
	repo       driven.RoomRepository
	peer       driven.WebRTCPeer
	translator driven.Translator
	codec      driven.AudioCodec
	notifier   driven.EventNotifier
	tts        driven.TextToSpeech // optional: nil disables voice output

	sessions  map[string]*session.Session // sessionID → Session
	lookup    map[string]string           // "roomID:userID" → sessionID
	pipelines map[string]*pipeline        // roomID → pipeline
	mu        sync.RWMutex

	graceTimers   map[string]*time.Timer // sessionID → pending grace timer
	graceTimersMu sync.Mutex
}

// NewService creates a new Service with the provided config and driven ports.
// Returns ErrNilDependency if any port is nil.
func NewService(cfg ServiceConfig, repo driven.RoomRepository, peer driven.WebRTCPeer, translator driven.Translator, codec driven.AudioCodec, notifier driven.EventNotifier) (*Service, error) {
	if repo == nil || peer == nil || translator == nil || codec == nil || notifier == nil {
		return nil, ErrNilDependency
	}
	if cfg.MaxShortCodeRetries <= 0 {
		cfg.MaxShortCodeRetries = 5
	}
	return &Service{
		cfg:         cfg,
		repo:        repo,
		peer:        peer,
		translator:  translator,
		codec:       codec,
		notifier:    notifier,
		sessions:    make(map[string]*session.Session),
		lookup:      make(map[string]string),
		pipelines:   make(map[string]*pipeline),
		graceTimers: make(map[string]*time.Timer),
	}, nil
}

// WithTTS sets the optional TextToSpeech dependency. When set, translated
// transcripts are synthesized to audio and sent to the target peer via WebRTC.
func (s *Service) WithTTS(tts driven.TextToSpeech) {
	s.tts = tts
}

// CreateRoom creates a new room with the given ISO 639-1 language codes.
// Generates a unique ShortCode with up to cfg.MaxShortCodeRetries collision retries.
// Returns a CreateRoomResult containing the new Room (with ID and ShortCode).
func (s *Service) CreateRoom(ctx context.Context, sourceLang, targetLang string) (driving.CreateRoomResult, error) {
	r, err := room.NewRoom(uuid.NewString(), sourceLang, targetLang)
	if err != nil {
		return driving.CreateRoomResult{}, fmt.Errorf("roomsvc.CreateRoom: %w", err)
	}

	// Generate unique short code with collision retry.
	for attempt := 0; attempt < s.cfg.MaxShortCodeRetries; attempt++ {
		code, err := room.GenerateShortCode(nil)
		if err != nil {
			return driving.CreateRoomResult{}, fmt.Errorf("roomsvc.CreateRoom: generating short code: %w", err)
		}
		existing, findErr := s.repo.FindByShortCode(ctx, code)
		if findErr != nil && !errors.Is(findErr, driving.ErrRoomNotFound) {
			return driving.CreateRoomResult{}, fmt.Errorf("roomsvc.CreateRoom: checking short code: %w", findErr)
		}
		if existing == nil {
			r.ShortCode = code
			break
		}
		// Collision — try again
	}

	if r.ShortCode == "" {
		return driving.CreateRoomResult{}, fmt.Errorf("roomsvc.CreateRoom: %w", room.ErrShortCodeExhausted)
	}

	if err := s.repo.Save(ctx, r); err != nil {
		return driving.CreateRoomResult{}, fmt.Errorf("roomsvc.CreateRoom: saving room: %w", err)
	}
	return driving.CreateRoomResult{Room: r}, nil
}

// DeleteRoom closes a room and cleans up all associated sessions.
// Returns ErrRoomNotFound if the room does not exist.
func (s *Service) DeleteRoom(ctx context.Context, roomID string) error {
	r, err := s.repo.FindByID(ctx, roomID)
	if err != nil {
		return fmt.Errorf("roomsvc.DeleteRoom: %w", err)
	}

	r.Close()

	s.mu.Lock()
	prefix := roomID + ":"
	for key, sessID := range s.lookup {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			if sess, ok := s.sessions[sessID]; ok {
				_ = sess.Disconnect()
				if err := s.peer.CloseSession(ctx, sessID); err != nil {
					slog.Error("close_session_error",
						"component", "service",
						"session_id", sessID,
						slog.Any("err", err))
				}
				delete(s.sessions, sessID)
			}
			delete(s.lookup, key)
		}
	}
	if p, ok := s.pipelines[roomID]; ok {
		p.cancel()
		delete(s.pipelines, roomID)
	}
	s.mu.Unlock()

	if err := s.repo.Delete(ctx, roomID); err != nil {
		return fmt.Errorf("roomsvc.DeleteRoom: %w", err)
	}
	return nil
}

// JoinRoom adds a user to an existing room and creates a WebRTC session.
// If a grace timer is active for this session's room, it cancels it (reconnection path).
// lang must be a non-empty ISO 639-1 code matching the room's SourceLang or TargetLang.
// Returns the new session ID on success.
func (s *Service) JoinRoom(ctx context.Context, roomID, userID, lang string) (string, error) {
	if lang == "" {
		return "", fmt.Errorf("roomsvc.JoinRoom: %w", ErrMissingLang)
	}

	r, err := s.repo.FindByID(ctx, roomID)
	if err != nil {
		return "", fmt.Errorf("roomsvc.JoinRoom: %w", err)
	}

	if lang != r.SourceLang && lang != r.TargetLang {
		return "", fmt.Errorf("roomsvc.JoinRoom: %w", ErrLangNotSupported)
	}

	// Check for reconnection: if user is already in room, treat as reconnect.
	joinErr := r.Join(userID)
	isReconnect := errors.Is(joinErr, room.ErrAlreadyInRoom)
	if joinErr != nil && !isReconnect {
		return "", fmt.Errorf("roomsvc.JoinRoom: %w", joinErr)
	}

	// Cancel any pending grace timer for this room BEFORE creating a new session,
	// so the room isn't deleted while we're rejoining.
	s.cancelGraceTimersForRoom(roomID)

	// On reconnect, look up the existing session ID.
	if isReconnect {
		s.mu.RLock()
		existingSessID, ok := s.lookup[roomID+":"+userID]
		s.mu.RUnlock()
		if ok {
			return existingSessID, nil
		}
		// Session not in map — fall through to create a new one.
		_ = r.Join(userID) // will fail again (ErrAlreadyInRoom) but we already checked; ignore
	}

	sess := session.NewSession(uuid.NewString(), roomID, userID, lang)

	if err := s.peer.CreateSession(ctx, sess.ID); err != nil {
		if !isReconnect {
			_ = r.Leave(userID)
		}
		return "", fmt.Errorf("roomsvc.JoinRoom: creating peer session: %w", err)
	}

	if err := s.repo.Save(ctx, r); err != nil {
		if !isReconnect {
			_ = r.Leave(userID)
		}
		_ = s.peer.CloseSession(ctx, sess.ID)
		return "", fmt.Errorf("roomsvc.JoinRoom: saving room: %w", err)
	}

	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.lookup[roomID+":"+userID] = sess.ID
	s.mu.Unlock()

	// Update LastActivity on join.
	r.TouchActivity()
	_ = s.repo.Save(ctx, r)

	if r.IsFull() {
		// Find the first participant's session (sessA) to launch the pipeline.
		var sessA, sessB *session.Session
		s.mu.RLock()
		for _, existingSess := range s.sessions {
			if existingSess.RoomID == roomID && existingSess.ID != sess.ID {
				sessA = existingSess
				break
			}
		}
		s.mu.RUnlock()
		sessB = sess

		if sessA != nil {
			go s.startPipeline(r, sessA, sessB)
		}
	}

	return sess.ID, nil
}

// cancelGraceTimersForRoom stops and removes all grace timers for sessions in the given room.
func (s *Service) cancelGraceTimersForRoom(roomID string) {
	s.mu.RLock()
	var sessIDsInRoom []string
	for _, sess := range s.sessions {
		if sess.RoomID == roomID {
			sessIDsInRoom = append(sessIDsInRoom, sess.ID)
		}
	}
	s.mu.RUnlock()

	s.graceTimersMu.Lock()
	for _, sid := range sessIDsInRoom {
		if t, ok := s.graceTimers[sid]; ok {
			t.Stop()
			delete(s.graceTimers, sid)
		}
	}
	s.graceTimersMu.Unlock()
}

// LeaveRoom disconnects a user from a room and cleans up their session.
func (s *Service) LeaveRoom(ctx context.Context, roomID, userID string) error {
	s.mu.RLock()
	sessID, ok := s.lookup[roomID+":"+userID]
	if !ok {
		s.mu.RUnlock()
		return fmt.Errorf("roomsvc.LeaveRoom: %w", driving.ErrSessionNotFound)
	}
	sess := s.sessions[sessID]
	s.mu.RUnlock()

	_ = sess.Disconnect()

	if err := s.peer.CloseSession(ctx, sessID); err != nil {
		slog.Error("close_session_error",
			"component", "service",
			"session_id", sessID,
			slog.Any("err", err))
	}

	r, err := s.repo.FindByID(ctx, roomID)
	if err != nil {
		return fmt.Errorf("roomsvc.LeaveRoom: %w", err)
	}

	if err := r.Leave(userID); err != nil && !errors.Is(err, room.ErrNotInRoom) {
		return fmt.Errorf("roomsvc.LeaveRoom: %w", err)
	}

	if err := s.repo.Save(ctx, r); err != nil {
		return fmt.Errorf("roomsvc.LeaveRoom: saving room: %w", err)
	}

	s.mu.Lock()
	delete(s.sessions, sessID)
	delete(s.lookup, roomID+":"+userID)
	if p, ok := s.pipelines[roomID]; ok {
		p.cancel()
		delete(s.pipelines, roomID)
	}
	s.mu.Unlock()

	slog.LogAttrs(ctx, slog.LevelInfo, "session_event",
		slog.String("event", "session_end"),
		slog.String("session_id", sessID),
		slog.String("room_id", roomID),
		slog.String("user_id", sess.UserID),
		slog.Int64("duration_sec", int64(time.Since(sess.JoinedAt).Seconds())),
		slog.String("event_type", "voluntary"),
		slog.String("component", "service"),
	)

	return nil
}

// RoomExists checks whether a room with the given ID exists.
// Returns ErrRoomNotFound (via repo) if the room does not exist.
func (s *Service) RoomExists(ctx context.Context, roomID string) error {
	_, err := s.repo.FindByID(ctx, roomID)
	if err != nil {
		return fmt.Errorf("roomsvc.RoomExists: %w", err)
	}
	return nil
}

// FindByShortCode looks up a room by its 6-char short code (case-insensitive).
func (s *Service) FindByShortCode(ctx context.Context, code string) (*room.Room, error) {
	r, err := s.repo.FindByShortCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("roomsvc.FindByShortCode: %w", err)
	}
	return r, nil
}

// UpdateLastActivity refreshes the LastActivity timestamp for the given room.
func (s *Service) UpdateLastActivity(ctx context.Context, roomID string) error {
	if err := s.repo.UpdateLastActivity(ctx, roomID); err != nil {
		return fmt.Errorf("roomsvc.UpdateLastActivity: %w", err)
	}
	return nil
}

// OnDisconnect is called by the Hub when a WebSocket client drops.
// If the sessionID maps to a known session, it starts a grace timer.
// When the timer fires, it deletes the room and notifies the remaining peer.
// If sessionID is empty or unknown, it is a no-op.
func (s *Service) OnDisconnect(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}

	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return nil // unknown session — no-op
	}

	roomID := sess.RoomID

	s.graceTimersMu.Lock()
	// Don't start a second timer if one is already running.
	if _, exists := s.graceTimers[sessionID]; exists {
		s.graceTimersMu.Unlock()
		return nil
	}

	// Emit session_end(disconnect) immediately.
	slog.LogAttrs(ctx, slog.LevelInfo, "session_event",
		slog.String("event", "session_end"),
		slog.String("session_id", sessionID),
		slog.String("room_id", roomID),
		slog.String("user_id", sess.UserID),
		slog.Int64("duration_sec", int64(time.Since(sess.JoinedAt).Seconds())),
		slog.String("event_type", "disconnect"),
		slog.String("component", "service"),
	)

	t := time.AfterFunc(s.cfg.GracePeriod, func() {
		s.graceTimersMu.Lock()
		delete(s.graceTimers, sessionID)
		s.graceTimersMu.Unlock()

		// Emit session_end(timeout) when the grace period expires.
		slog.LogAttrs(context.Background(), slog.LevelInfo, "session_event",
			slog.String("event", "session_end"),
			slog.String("session_id", sessionID),
			slog.String("room_id", roomID),
			slog.String("user_id", sess.UserID),
			slog.Int64("duration_sec", int64(time.Since(sess.JoinedAt).Seconds())),
			slog.String("event_type", "timeout"),
			slog.String("component", "service"),
		)

		// Delete room and notify peers.
		if err := s.DeleteRoom(ctx, roomID); err != nil && !errors.Is(err, driving.ErrRoomNotFound) {
			slog.Error("grace_timer_delete_error",
				"component", "service",
				"room_id", roomID,
				slog.Any("err", err))
		}
		s.notifier.NotifySession(sessionID, "room-closed", map[string]string{
			"reason": "peer-timeout",
		})
	})
	s.graceTimers[sessionID] = t
	s.graceTimersMu.Unlock()

	return nil
}

// StartExpirationSweep starts a background goroutine that periodically deletes
// rooms that have been inactive for longer than cfg.RoomTTL.
// The goroutine exits when ctx is cancelled.
func (s *Service) StartExpirationSweep(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(s.cfg.SweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sweepExpiredRooms(ctx)
			}
		}
	}()
}

func (s *Service) sweepExpiredRooms(ctx context.Context) {
	cutoff := time.Now().Add(-s.cfg.RoomTTL)
	expired, err := s.repo.ListExpired(ctx, cutoff)
	if err != nil {
		slog.Error("sweep_list_error", "component", "service", slog.Any("err", err))
		return
	}
	for _, r := range expired {
		if err := s.DeleteRoom(ctx, r.ID); err != nil && !errors.Is(err, driving.ErrRoomNotFound) {
			slog.Error("sweep_delete_error",
				"component", "service",
				"room_id", r.ID,
				slog.Any("err", err))
		}
	}
}

// HandleSignaling dispatches a typed signaling message and returns the response.
// Implements driving.SignalingHandler.
func (s *Service) HandleSignaling(ctx context.Context, msg driving.SignalingMessage) (driving.SignalingMessage, error) { //nolint:gocritic // SignalingMessage is a DTO; value semantics are intentional
	switch msg.Type {
	case "join":
		sessID, err := s.JoinRoom(ctx, msg.RoomID, msg.UserID, msg.Lang)
		if err != nil {
			return driving.SignalingMessage{Type: "error", Message: err.Error()}, nil
		}
		return driving.SignalingMessage{Type: "joined", SessionID: sessID, RoomID: msg.RoomID}, nil

	case "offer":
		if err := s.peer.HandleOffer(ctx, msg.SessionID, msg.SDP); err != nil {
			return driving.SignalingMessage{Type: "error", Message: err.Error()}, nil
		}
		// Register trickle ICE callback before gathering starts (CreateAnswer triggers gathering).
		_ = s.peer.OnICECandidate(ctx, msg.SessionID, func(candidate string) {
			s.notifier.NotifySession(msg.SessionID, "ice-candidate", map[string]string{
				"candidate": candidate,
			})
		})
		answer, err := s.peer.CreateAnswer(ctx, msg.SessionID)
		if err != nil {
			return driving.SignalingMessage{Type: "error", Message: err.Error()}, nil
		}
		return driving.SignalingMessage{Type: "answer", SessionID: msg.SessionID, SDP: answer}, nil

	case "ice-candidate":
		_ = s.peer.AddICECandidate(ctx, msg.SessionID, msg.Candidate)
		return driving.SignalingMessage{}, nil

	case "leave":
		s.mu.RLock()
		sess, ok := s.sessions[msg.SessionID]
		s.mu.RUnlock()
		if !ok {
			return driving.SignalingMessage{Type: "error", Message: "session not found"}, nil
		}
		_ = s.LeaveRoom(ctx, sess.RoomID, sess.UserID)
		return driving.SignalingMessage{}, nil

	default:
		return driving.SignalingMessage{}, driving.ErrUnknownMessageType
	}
}
