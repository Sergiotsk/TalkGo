// Package roomsvc implements the application layer that orchestrates domain
// logic and driven ports to fulfil the RoomManager and SignalingHandler contracts.
package roomsvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"github.com/Sergiotsk/TalkGo/internal/domain/room"
	"github.com/Sergiotsk/TalkGo/internal/domain/session"
	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
	"github.com/Sergiotsk/TalkGo/internal/ports/driving"
)

// Service implements driving.RoomManager and driving.SignalingHandler.
type Service struct {
	repo       driven.RoomRepository
	peer       driven.WebRTCPeer
	translator driven.Translator
	codec      driven.AudioCodec
	notifier   driven.EventNotifier
	sessions   map[string]*session.Session // sessionID → Session
	lookup     map[string]string           // "roomID:userID" → sessionID
	pipelines  map[string]*pipeline        // roomID → pipeline
	mu         sync.RWMutex
}

// NewService creates a new Service with the provided driven ports.
// Returns ErrNilDependency if any port is nil.
func NewService(repo driven.RoomRepository, peer driven.WebRTCPeer, translator driven.Translator, codec driven.AudioCodec, notifier driven.EventNotifier) (*Service, error) {
	if repo == nil || peer == nil || translator == nil || codec == nil || notifier == nil {
		return nil, ErrNilDependency
	}
	return &Service{
		repo:       repo,
		peer:       peer,
		translator: translator,
		codec:      codec,
		notifier:   notifier,
		sessions:   make(map[string]*session.Session),
		lookup:     make(map[string]string),
		pipelines:  make(map[string]*pipeline),
	}, nil
}

// CreateRoom creates a new room with the given ISO 639-1 language codes.
// Returns the new room ID on success.
func (s *Service) CreateRoom(ctx context.Context, sourceLang, targetLang string) (string, error) {
	r, err := room.NewRoom(uuid.NewString(), sourceLang, targetLang)
	if err != nil {
		return "", fmt.Errorf("roomsvc.CreateRoom: %w", err)
	}
	if err := s.repo.Save(ctx, r); err != nil {
		return "", fmt.Errorf("roomsvc.CreateRoom: saving room: %w", err)
	}
	return r.ID, nil
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
					slog.Error("closing peer session on room delete",
						slog.String("sessionID", sessID),
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
// lang must be a non-empty ISO 639-1 code matching the room's SourceLang or TargetLang.
// Returns the new session ID on success.
// Spec invariants (in order):
//  1. Validate lang — ErrMissingLang if empty; ErrLangNotSupported if not room language.
//  2. Find room — ErrRoomNotFound if missing.
//  3. room.Join — propagate domain errors.
//  4. Create Session with lang.
//  5. peer.CreateSession — on failure, rollback room.Leave.
//  6. repo.Save — on failure, rollback room.Leave + peer.CloseSession.
//  7. Store session internally.
//  8. If room is full, launch startPipeline goroutine.
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

	if err := r.Join(userID); err != nil {
		return "", fmt.Errorf("roomsvc.JoinRoom: %w", err)
	}

	sess := session.NewSession(uuid.NewString(), roomID, userID, lang)

	if err := s.peer.CreateSession(ctx, sess.ID); err != nil {
		_ = r.Leave(userID)
		return "", fmt.Errorf("roomsvc.JoinRoom: creating peer session: %w", err)
	}

	if err := s.repo.Save(ctx, r); err != nil {
		_ = r.Leave(userID)
		_ = s.peer.CloseSession(ctx, sess.ID)
		return "", fmt.Errorf("roomsvc.JoinRoom: saving room: %w", err)
	}

	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.lookup[roomID+":"+userID] = sess.ID
	s.mu.Unlock()

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

// LeaveRoom disconnects a user from a room and cleans up their session.
// Spec invariants (in order):
//  1. Lookup session by roomID+userID — ErrSessionNotFound if missing.
//  2. session.Disconnect.
//  3. peer.CloseSession — log error, do not abort.
//  4. room.Leave — propagate unless ErrNotInRoom (idempotent).
//  5. repo.Save.
//  6. Remove session from internal maps.
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
		slog.Error("closing peer session",
			slog.String("sessionID", sessID),
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
	// Cancel the active pipeline for this room if present — the remaining
	// participant can no longer be part of a full-room session.
	if p, ok := s.pipelines[roomID]; ok {
		p.cancel()
		delete(s.pipelines, roomID)
	}
	s.mu.Unlock()

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
