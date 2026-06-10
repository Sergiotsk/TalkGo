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
	cfg     Config
	manager driving.RoomManager
	hub     *signaling.Hub
	mux     *http.ServeMux
}

// NewServer creates a Server and registers all routes.
func NewServer(cfg Config, manager driving.RoomManager, hub *signaling.Hub) *Server {
	s := &Server{
		cfg:     cfg,
		manager: manager,
		hub:     hub,
		mux:     http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Handler returns the http.Handler for use in tests.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /health", s.healthHandler)
	s.mux.HandleFunc("POST /rooms", s.createRoomHandler)
	s.mux.HandleFunc("DELETE /rooms/{id}", s.deleteRoomHandler)
	s.mux.HandleFunc("GET /ws/{roomID}", s.wsHandler)
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
		slog.Info("server starting", slog.String("addr", s.cfg.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", slog.Any("err", err))
		}
	}()

	<-quit
	slog.Info("shutting down server")

	shutCtx, cancel := context.WithTimeout(ctx, s.cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		return err
	}
	slog.Info("server stopped")
	return nil
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (s *Server) healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

	roomID, err := s.manager.CreateRoom(r.Context(), req.SourceLang, req.TargetLang)
	if err != nil {
		if isLanguageError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		slog.Error("createRoom", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"room_id": roomID})
}

func (s *Server) deleteRoomHandler(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("id")
	if err := s.manager.DeleteRoom(r.Context(), roomID); err != nil {
		if errors.Is(err, driving.ErrRoomNotFound) {
			writeError(w, http.StatusNotFound, "room not found")
			return
		}
		slog.Error("deleteRoom", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
		slog.Error("wsHandler roomExists", slog.Any("err", err))
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
