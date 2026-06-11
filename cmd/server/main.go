package main

import (
	"context"
	"log/slog"
	"os"

	codecadapter "github.com/Sergiotsk/TalkGo/internal/adapters/codec"
	httpserver "github.com/Sergiotsk/TalkGo/internal/adapters/http"
	"github.com/Sergiotsk/TalkGo/internal/adapters/signaling"
	"github.com/Sergiotsk/TalkGo/internal/adapters/translator"
	webrtcadapter "github.com/Sergiotsk/TalkGo/internal/adapters/webrtc"
	"github.com/Sergiotsk/TalkGo/internal/app/roomsvc"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Driven adapters
	peer := webrtcadapter.NewPionPeer(webrtcadapter.DefaultConfig())
	repo := roomsvc.NewInMemoryRoomRepository()
	tr := translator.NewOpenAIRealtimeTranslator(translator.OpenAIRealtimeConfig{
		APIKey: os.Getenv("OPENAI_API_KEY"),
		Model:  "gpt-4o-realtime-preview",
	})
	codec := codecadapter.NewPassthroughCodec()

	// Break the circular dependency: Hub needs svc as SignalingHandler,
	// svc needs Hub as EventNotifier. Wire hub first with nil handler,
	// then inject svc after construction.
	hub := signaling.NewHub(nil)

	svc, err := roomsvc.NewService(repo, peer, tr, codec, hub)
	if err != nil {
		slog.Error("creating service", slog.Any("err", err))
		os.Exit(1)
	}

	// Complete the circular wire: give hub its SignalingHandler.
	hub.SetHandler(svc)

	go hub.Run()

	// HTTP server
	srv := httpserver.NewServer(httpserver.DefaultConfig(), svc, hub)

	slog.Info("TalkGo starting")
	if err := srv.ListenAndServe(context.Background()); err != nil {
		slog.Error("server error", slog.Any("err", err))
		os.Exit(1)
	}
}
