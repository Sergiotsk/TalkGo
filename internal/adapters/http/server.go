// Package httpserver implements the HTTP and WebSocket adapter.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Sergiotsk/TalkGo/internal/adapters/signaling"
	"github.com/Sergiotsk/TalkGo/internal/domain/room"
	"github.com/Sergiotsk/TalkGo/internal/ports/driving"
)

// Config holds HTTP server configuration.
type Config struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
	// Phase 7 — REQ-OPS-06: health check extension fields.
	TurnConfigured bool
	APIKeyPresent  bool
	CodecMode      string
}

// DefaultConfig returns sensible defaults for local development.
func DefaultConfig() Config {
	return Config{
		Addr:            ":8080",
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    10 * time.Second,
		ShutdownTimeout: 15 * time.Second,
	}
}

// Server is the HTTP adapter. It wires routes to the RoomManager driving port
// and the WebSocket signaling Hub.
type Server struct {
	cfg         Config
	manager     driving.RoomManager
	hub         *signaling.Hub
	mux         *http.ServeMux
	roomLimiter *RateLimiter
	wsLimiter   *RateLimiter
}

// Addr returns the configured listen address.
func (s *Server) Addr() string { return s.cfg.Addr }

// NewServer creates a Server and registers all routes.
// roomLimiter and wsLimiter may be nil (no rate limiting for that endpoint).
func NewServer(cfg Config, manager driving.RoomManager, hub *signaling.Hub, roomLimiter, wsLimiter *RateLimiter) *Server {
	s := &Server{
		cfg:         cfg,
		manager:     manager,
		hub:         hub,
		mux:         http.NewServeMux(),
		roomLimiter: roomLimiter,
		wsLimiter:   wsLimiter,
	}
	s.registerRoutes()
	return s
}

// Handler returns the http.Handler for use in tests.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /health", s.healthHandler)
	s.mux.HandleFunc("DELETE /rooms/{id}", s.deleteRoomHandler)
	s.mux.HandleFunc("GET /rooms/code/{code}", s.getRoomByShortCodeHandler)
	s.mux.HandleFunc("POST /feedback", s.feedbackHandler) // TASK-046

	// POST /rooms — wrap with rate limiter when configured (TASK-055).
	roomsHandler := http.HandlerFunc(s.createRoomHandler)
	if s.roomLimiter != nil {
		s.mux.Handle("POST /rooms", s.roomLimiter.Middleware(roomsHandler))
	} else {
		s.mux.Handle("POST /rooms", roomsHandler)
	}

	// GET /ws/{roomID} — wrap with rate limiter when configured (TASK-055).
	wsHandler := http.HandlerFunc(s.wsHandler)
	if s.wsLimiter != nil {
		s.mux.Handle("GET /ws/{roomID}", s.wsLimiter.Middleware(wsHandler))
	} else {
		s.mux.Handle("GET /ws/{roomID}", wsHandler)
	}
}

// ListenAndServe starts the server and blocks until a shutdown signal is received.
func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:         s.cfg.Addr,
		Handler:      s.mux,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("http_listening", "component", "http", "addr", s.cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http_listening_error", "component", "http", slog.Any("err", err))
		}
	}()

	<-quit
	slog.Info("http_shutdown", "component", "http")

	shutCtx, cancel := context.WithTimeout(ctx, s.cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		return err
	}
	slog.Info("http_stopped", "component", "http")
	return nil
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (s *Server) healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":          "ok",
		"turn_configured": s.cfg.TurnConfigured,
		"api_key_present": s.cfg.APIKeyPresent,
		"codec_mode":      s.cfg.CodecMode,
	})
}

func (s *Server) createRoomHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceLang string `json:"source_lang"`
		TargetLang string `json:"target_lang"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := s.manager.CreateRoom(r.Context(), req.SourceLang, req.TargetLang)
	if err != nil {
		if isLanguageError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, room.ErrRoomFull) {
			writeError(w, http.StatusConflict, "Esta sala ya tiene 2 participants")
			return
		}
		if errors.Is(err, room.ErrRoomClosed) {
			writeError(w, http.StatusGone, "Esta sala expiró. Creá una nueva.")
			return
		}
		slog.Error("create_room_error", "component", "http", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"room_id":    result.Room.ID,
		"short_code": result.Room.ShortCode,
	})
}

func (s *Server) deleteRoomHandler(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("id")
	if err := s.manager.DeleteRoom(r.Context(), roomID); err != nil {
		if errors.Is(err, driving.ErrRoomNotFound) {
			writeError(w, http.StatusNotFound, "room not found")
			return
		}
		slog.Error("delete_room_error", "component", "http", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getRoomByShortCodeHandler handles GET /rooms/code/{code}.
// Returns 200 with room info, 404 if not found, 410 if the room is closed/expired.
func (s *Server) getRoomByShortCodeHandler(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	rm, err := s.manager.FindByShortCode(r.Context(), code)
	if err != nil {
		if errors.Is(err, driving.ErrRoomNotFound) {
			writeError(w, http.StatusNotFound, "room not found")
			return
		}
		if errors.Is(err, room.ErrRoomClosed) {
			writeError(w, http.StatusGone, "Esta sala expiró. Creá una nueva.")
			return
		}
		slog.Error("find_by_code_error", "component", "http", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"room_id":    rm.ID,
		"short_code": rm.ShortCode,
	})
}

func (s *Server) wsHandler(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("roomID")
	if roomID == "" {
		writeError(w, http.StatusBadRequest, "roomID is required")
		return
	}
	if err := s.manager.RoomExists(r.Context(), roomID); err != nil {
		if errors.Is(err, driving.ErrRoomNotFound) {
			writeError(w, http.StatusNotFound, "room not found")
			return
		}
		slog.Error("ws_handler_error", "component", "http", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if s.hub == nil {
		writeError(w, http.StatusInternalServerError, "signaling not configured")
		return
	}
	s.hub.ServeWS(w, r, roomID)
}

// ---------------------------------------------------------------------------
// TASK-045: feedbackRequest + feedbackHandler
// ---------------------------------------------------------------------------

// feedbackRequest is the payload for POST /feedback.
type feedbackRequest struct {
	SessionID string `json:"session_id"`
	Rating    int    `json:"rating"`
	Comment   string `json:"comment"`
}

// feedbackHandler handles POST /feedback.
// Validates session_id and rating, truncates comment at 1000 chars, logs and
// returns 200 + {"status":"ok"} on success or 400 + {"error":"..."} on failure.
func (s *Server) feedbackHandler(w http.ResponseWriter, r *http.Request) {
	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		writeError(w, http.StatusBadRequest, "rating must be between 1 and 5")
		return
	}

	comment := req.Comment
	if len([]rune(comment)) > 1000 {
		comment = string([]rune(comment)[:1000])
	}

	slog.Info("user_feedback",
		slog.String("session_id", req.SessionID),
		slog.Int("rating", req.Rating),
		slog.String("comment", comment),
	)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// isLanguageError detects language-code validation errors from the domain sentinel.
func isLanguageError(err error) bool {
	return errors.Is(err, room.ErrInvalidLanguageCode)
}
