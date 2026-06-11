package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	codecadapter "github.com/Sergiotsk/TalkGo/internal/adapters/codec"
	httpserver "github.com/Sergiotsk/TalkGo/internal/adapters/http"
	"github.com/Sergiotsk/TalkGo/internal/adapters/signaling"
	"github.com/Sergiotsk/TalkGo/internal/adapters/translator"
	webrtcadapter "github.com/Sergiotsk/TalkGo/internal/adapters/webrtc"
	"github.com/Sergiotsk/TalkGo/internal/app/roomsvc"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown on SIGINT/SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		slog.Info("shutdown signal received")
		cancel()
	}()

	// Driven adapters
	peer := webrtcadapter.NewPionPeer(webrtcadapter.DefaultConfig())
	repo := roomsvc.NewInMemoryRoomRepository()
	tr := translator.NewOpenAIRealtimeTranslator(translator.OpenAIRealtimeConfig{
		APIKey: os.Getenv("OPENAI_API_KEY"),
		Model:  "gpt-4o-realtime-preview",
	})
	codec := codecadapter.NewPassthroughCodec()

	// Service configuration
	cfg := roomsvc.ServiceConfig{
		GracePeriod:         30 * time.Second,
		RoomTTL:             10 * time.Minute,
		SweepInterval:       60 * time.Second,
		MaxShortCodeRetries: 5,
	}

	// Break the circular dependency: Hub needs svc as SignalingHandler,
	// svc needs Hub as EventNotifier. Wire hub first with nil handler,
	// then inject svc after construction.
	hub := signaling.NewHub(nil)

	svc, err := roomsvc.NewService(cfg, repo, peer, tr, codec, hub)
	if err != nil {
		slog.Error("creating service", slog.Any("err", err))
		return 1
	}

	// Complete the circular wire: give hub its SignalingHandler.
	hub.SetHandler(svc)

	// Start hub with context for graceful shutdown.
	go hub.RunCtx(ctx)

	// Start expiration sweep goroutine.
	svc.StartExpirationSweep(ctx)

	// HTTP server
	srv := httpserver.NewServer(httpserver.DefaultConfig(), svc, hub)

	slog.Info("TalkGo starting")
	if err := srv.ListenAndServe(ctx); err != nil {
		slog.Error("server error", slog.Any("err", err))
		return 1
	}

	return 0
}
