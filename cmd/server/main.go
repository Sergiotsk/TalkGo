package main

import (
	"context"
	"log/slog"
	"os"

	httpserver "github.com/Sergiotsk/TalkGo/internal/adapters/http"
	"github.com/Sergiotsk/TalkGo/internal/adapters/signaling"
	webrtcadapter "github.com/Sergiotsk/TalkGo/internal/adapters/webrtc"
	"github.com/Sergiotsk/TalkGo/internal/app/roomsvc"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Driven adapters
	peer := webrtcadapter.NewPionPeer(webrtcadapter.DefaultConfig())
	repo := roomsvc.NewInMemoryRoomRepository()

	// App layer (implements RoomManager + SignalingHandler)
	svc := roomsvc.NewService(repo, peer)

	// Signaling hub
	hub := signaling.NewHub(svc)
	go hub.Run()

	// HTTP server
	srv := httpserver.NewServer(httpserver.DefaultConfig(), svc, hub)

	slog.Info("TalkGo starting")
	if err := srv.ListenAndServe(context.Background()); err != nil {
		slog.Error("server error", slog.Any("err", err))
		os.Exit(1)
	}
}
