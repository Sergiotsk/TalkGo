// Command loadgen is a WebSocket-based load generator for TalkGo server testing.
//
// It connects to a TalkGo server as a simulated peer, follows the signaling
// protocol (join -> offer/answer), measures round-trip latency via WebSocket
// ping/pong, and emits a JSON performance report.
//
// Usage:
//
//	loadgen -server localhost:8080 -duration 30s -profile 4g
//
// Flags:
//
//	-server    TalkGo server address (default "localhost:8080")
//	-room      Room ID (empty = auto-create via POST /rooms)
//	-lang      Peer language (default "es")
//	-duration  Test duration (default 30s)
//	-profile   Network profile label for report (default "wifi-home")
//	-output    Report output path (default: stdout JSON)
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	os.Exit(run())
}

func run() int {
	server := flag.String("server", "localhost:8080", "TalkGo server address")
	roomID := flag.String("room", "", "Room ID (empty = auto-create via HTTP)")
	lang := flag.String("lang", "es", "Peer language code")
	duration := flag.Duration("duration", 30*time.Second, "Test duration")
	profile := flag.String("profile", "wifi-home", "Network profile label for report")
	output := flag.String("output", "", "Report output file (default: stdout)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `TalkGo Load Generator - WebSocket load testing tool

Usage:
  loadgen [flags]

Flags:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  # 30-second test with default settings
  loadgen -duration 30s

  # Test against custom server with specific room
  loadgen -server 10.0.0.5:8080 -room abc123 -duration 60s -profile 4g

  # Save report to file
  loadgen -duration 10s -output report.json
`)
	}
	flag.Parse()

	if *duration <= 0 {
		fmt.Fprintf(os.Stderr, "error: -duration must be positive, got %v\n", *duration)
		return 1
	}

	// Resolve room: auto-create via HTTP if not provided.
	rid := *roomID
	if rid == "" {
		var err error
		rid, err = createRoom(fmt.Sprintf("http://%s/rooms", *server), *lang)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: creating room: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "created room: %s\n", rid)
	}

	// Build the WebSocket URL.
	wsURL := fmt.Sprintf("ws://%s/ws/%s", *server, rid)

	// Run the load session.
	report, err := runSession(wsURL, rid, *lang, *duration, *profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: session failed: %v\n", err)
		if report == nil {
			return 1
		}
		report.Errors = append(report.Errors, err.Error())
	}

	// Serialise and output the report.
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: marshalling report: %v\n", err)
		return 1
	}

	if *output != "" {
		if err := os.WriteFile(*output, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error: writing report: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "report written to %s\n", *output)
	} else {
		fmt.Println(string(data))
	}

	if report.Status == "failed" {
		return 1
	}
	return 0
}

// createRoom creates a room via the TalkGo HTTP API and returns the room ID.
func createRoom(url, lang string) (string, error) {
	body := fmt.Sprintf(`{"source_lang":"%s","target_lang":"en"}`, lang)
	resp, err := http.Post(url, "application/json", bytes.NewReader([]byte(body))) //nolint:noctx // short-lived
	if err != nil {
		return "", fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("POST %s: %s: %s", url, resp.Status, string(data))
	}

	var result struct {
		RoomID    string `json:"room_id"`
		ShortCode string `json:"short_code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}
	if result.RoomID == "" {
		return "", fmt.Errorf("server returned empty room_id")
	}
	return result.RoomID, nil
}
