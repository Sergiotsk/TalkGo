package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	codecadapter "github.com/Sergiotsk/TalkGo/internal/adapters/codec"
	httpserver "github.com/Sergiotsk/TalkGo/internal/adapters/http"
	"github.com/Sergiotsk/TalkGo/internal/adapters/signaling"
	"github.com/Sergiotsk/TalkGo/internal/adapters/translator"
	webrtcadapter "github.com/Sergiotsk/TalkGo/internal/adapters/webrtc"
	"github.com/Sergiotsk/TalkGo/internal/app/roomsvc"
	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
)

// ---------------------------------------------------------------------------
// TASK-069: appConfig + loadConfig
// ---------------------------------------------------------------------------

// appConfig holds all configuration derived from environment variables.
type appConfig struct {
	Port           string
	LogLevel       string
	CodecMode      string
	OpenAIAPIKey   string
	TurnURLs       string
	TurnUsername   string
	TurnPassword   string
	RateLimitRooms int
	RateLimitWS    int
}

// loadConfig reads all environment variables, validates required ones, and
// returns a populated appConfig or an error.
//
// Required:
//   - OPENAI_API_KEY — returns error if empty (REQ-OPS-03)
//
// Validated:
//   - CODEC_MODE — must be "opus" or "passthrough"; default "opus" (REQ-COD-03)
//
// Soft-parsed (default on parse error, no error returned):
//   - RATE_LIMIT_ROOMS — default 10 (REQ-RATE-03)
//   - RATE_LIMIT_WS    — default 20 (REQ-RATE-03)
func loadConfig() (appConfig, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return appConfig{}, fmt.Errorf("OPENAI_API_KEY is required but not set")
	}

	codecMode := os.Getenv("CODEC_MODE")
	if codecMode == "" {
		codecMode = "opus"
	}
	if codecMode != "opus" && codecMode != "passthrough" {
		return appConfig{}, fmt.Errorf("CODEC_MODE=%q is invalid: must be \"opus\" or \"passthrough\"", codecMode)
	}

	rateLimitRooms := 10
	if v := os.Getenv("RATE_LIMIT_ROOMS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Warn("config_parse_warning", "field", "RATE_LIMIT_ROOMS", "value", v, "default", rateLimitRooms)
		} else {
			rateLimitRooms = n
		}
	}

	rateLimitWS := 20
	if v := os.Getenv("RATE_LIMIT_WS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Warn("config_parse_warning", "field", "RATE_LIMIT_WS", "value", v, "default", rateLimitWS)
		} else {
			rateLimitWS = n
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logLevelEnv := os.Getenv("LOG_LEVEL")
	if logLevelEnv == "" {
		logLevelEnv = "info"
	}

	return appConfig{
		Port:           port,
		LogLevel:       logLevelEnv,
		CodecMode:      codecMode,
		OpenAIAPIKey:   apiKey,
		TurnURLs:       os.Getenv("TURN_URLS"),
		TurnUsername:   os.Getenv("TURN_USERNAME"),
		TurnPassword:   os.Getenv("TURN_PASSWORD"),
		RateLimitRooms: rateLimitRooms,
		RateLimitWS:    rateLimitWS,
	}, nil
}

func main() {
	os.Exit(run())
}

// parseLogLevel converts a string level name to slog.Level.
// Returns slog.LevelInfo for unknown values.
func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// setupLogger creates a JSON logger writing to w at the given level.
func setupLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}

func run() int {
	flag.Parse()

	// TASK-070: Load all config from environment before anything else.
	appCfg, err := loadConfig()
	if err != nil {
		// Logger may not be set up yet — use the default slog handler.
		slog.Error("config_load_failed", "component", "main", slog.Any("err", err))
		return 1
	}

	logger := setupLogger(os.Stdout, parseLogLevel(appCfg.LogLevel))
	slog.SetDefault(logger)

	slog.Info("config_loaded",
		"codec_mode", appCfg.CodecMode,
		"turn_configured", appCfg.TurnURLs != "",
		"rate_limit_rooms", appCfg.RateLimitRooms,
		"rate_limit_ws", appCfg.RateLimitWS,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown on SIGINT/SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		slog.Info("shutdown_starting", "component", "main")
		cancel()
	}()

	// TASK-070: Build ICE config from env-driven TURN settings (REQ-NET-02).
	iceConfig := webrtcadapter.BuildICEConfig(appCfg.TurnURLs, appCfg.TurnUsername, appCfg.TurnPassword)
	peer := webrtcadapter.NewPionPeer(iceConfig)

	// TASK-070: Select codec based on CODEC_MODE (REQ-COD-03).
	var audioCodec driven.AudioCodec
	switch appCfg.CodecMode {
	case "opus":
		audioCodec = codecadapter.NewOpusCodec()
	default: // "passthrough"
		audioCodec = codecadapter.NewPassthroughCodec()
	}

	// TASK-070: Create rate limiters from env-driven limits (REQ-RATE-03).
	roomLimiter := httpserver.NewRateLimiter(appCfg.RateLimitRooms, time.Hour)
	wsLimiter := httpserver.NewRateLimiter(appCfg.RateLimitWS, time.Hour)

	// Start cleanup goroutines for rate limiters.
	roomLimiter.StartCleanup(ctx, 5*time.Minute)
	wsLimiter.StartCleanup(ctx, 5*time.Minute)

	repo := roomsvc.NewInMemoryRoomRepository()
	tr := translator.NewOpenAIRealtimeTranslator(translator.OpenAIRealtimeConfig{
		APIKey: appCfg.OpenAIAPIKey,
		Model:  "gpt-4o-realtime-preview",
	})

	// Service configuration
	svcCfg := roomsvc.ServiceConfig{
		GracePeriod:         30 * time.Second,
		RoomTTL:             10 * time.Minute,
		SweepInterval:       60 * time.Second,
		MaxShortCodeRetries: 5,
	}

	// Break the circular dependency: Hub needs svc as SignalingHandler,
	// svc needs Hub as EventNotifier. Wire hub first with nil handler,
	// then inject svc after construction.
	hub := signaling.NewHub(nil)

	svc, err := roomsvc.NewService(svcCfg, repo, peer, tr, audioCodec, hub)
	if err != nil {
		slog.Error("service_creation_failed", "component", "main", slog.Any("err", err))
		return 1
	}

	// Complete the circular wire: give hub its SignalingHandler.
	hub.SetHandler(svc)

	// TASK-071: Wire OnICEFailed callback — notifies the session client via hub (REQ-UX-01, REQ-MOB-03).
	peer.OnICEFailed = func(sessionID string) {
		hub.NotifySession(sessionID, "error", map[string]string{
			"code":       "ice-failed",
			"message":    "ICE connection failed",
			"session_id": sessionID,
		})
	}

	// Start hub with context for graceful shutdown.
	go hub.RunCtx(ctx)

	// Start expiration sweep goroutine.
	svc.StartExpirationSweep(ctx)

	// TASK-070: Build HTTP server config with Sprint 5 fields (REQ-OPS-06).
	httpCfg := httpserver.DefaultConfig()
	httpCfg.Addr = ":" + appCfg.Port
	httpCfg.TurnConfigured = appCfg.TurnURLs != ""
	httpCfg.APIKeyPresent = appCfg.OpenAIAPIKey != ""
	httpCfg.CodecMode = appCfg.CodecMode

	srv := httpserver.NewServer(httpCfg, svc, hub, roomLimiter, wsLimiter)

	slog.Info("server_starting", "component", "main", "addr", srv.Addr())
	if err := srv.ListenAndServe(ctx); err != nil {
		slog.Error("server_error", "component", "http", slog.Any("err", err))
		return 1
	}

	return 0
}
