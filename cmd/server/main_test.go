package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestJSONHandler_ConfiguredAtStartup(t *testing.T) {
	// setupLogger must return a JSON handler.
	var buf bytes.Buffer
	logger := setupLogger(&buf, slog.LevelInfo)

	// Emit a simple log message.
	logger.Info("server_starting", "component", "main", "addr", ":8080")

	// Verify it's valid JSON.
	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected log output, got empty")
	}

	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("expected valid JSON log, got: %s", line)
	}

	if entry["msg"] != "server_starting" {
		t.Errorf("expected msg=server_starting, got %v", entry["msg"])
	}
	if entry["component"] != "main" {
		t.Errorf("expected component=main, got %v", entry["component"])
	}
	if entry["addr"] != ":8080" {
		t.Errorf("expected addr=:8080, got %v", entry["addr"])
	}
}

func TestLogLevel_Debug(t *testing.T) {
	// With LevelInfo, a Debug message should NOT appear.
	var bufInfo bytes.Buffer
	loggerInfo := setupLogger(&bufInfo, slog.LevelInfo)
	loggerInfo.Debug("should_not_appear")
	if bufInfo.Len() > 0 {
		t.Error("expected no output for Debug at Info level")
	}

	// With LevelDebug, a Debug message SHOULD appear.
	var bufDebug bytes.Buffer
	loggerDebug := setupLogger(&bufDebug, slog.LevelDebug)
	loggerDebug.Debug("should_appear")
	if bufDebug.Len() == 0 {
		t.Error("expected output for Debug at Debug level")
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}

	for _, tt := range tests {
		got := parseLogLevel(tt.input)
		if got != tt.want {
			t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// TASK-065: loadConfig — missing OPENAI_API_KEY returns error (REQ-OPS-03)
// ---------------------------------------------------------------------------

func TestLoadConfig_MissingAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	_, err := loadConfig()
	if err == nil {
		t.Fatal("expected error when OPENAI_API_KEY is unset, got nil")
	}
}

// ---------------------------------------------------------------------------
// TASK-066: loadConfig — invalid CODEC_MODE returns error (REQ-COD-03)
// ---------------------------------------------------------------------------

func TestLoadConfig_InvalidCodecMode(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("CODEC_MODE", "invalid")

	_, err := loadConfig()
	if err == nil {
		t.Fatal("expected error when CODEC_MODE=invalid, got nil")
	}

	t.Setenv("CODEC_MODE", "")
}

// ---------------------------------------------------------------------------
// TASK-067: loadConfig — invalid RATE_LIMIT_ROOMS falls back to default 10 (REQ-RATE-03)
// ---------------------------------------------------------------------------

func TestLoadConfig_InvalidRateLimit_UsesDefault(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("CODEC_MODE", "passthrough")
	t.Setenv("RATE_LIMIT_ROOMS", "abc")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("expected no error for invalid RATE_LIMIT_ROOMS, got: %v", err)
	}
	if cfg.RateLimitRooms != 10 {
		t.Errorf("RateLimitRooms = %d, want 10 (default)", cfg.RateLimitRooms)
	}

	t.Setenv("RATE_LIMIT_ROOMS", "")
}

// ---------------------------------------------------------------------------
// TASK-068: loadConfig — all env vars set, all fields populated (REQ-OPS-03)
// ---------------------------------------------------------------------------

func TestLoadConfig_AllVarsSet(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-real-key")
	t.Setenv("CODEC_MODE", "passthrough")
	t.Setenv("PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("TURN_URLS", "turn:myturn.example.com:3478")
	t.Setenv("TURN_USERNAME", "turnuser")
	t.Setenv("TURN_PASSWORD", "turnpass")
	t.Setenv("RATE_LIMIT_ROOMS", "5")
	t.Setenv("RATE_LIMIT_WS", "15")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.OpenAIAPIKey != "sk-real-key" {
		t.Errorf("OpenAIAPIKey = %q, want %q", cfg.OpenAIAPIKey, "sk-real-key")
	}
	if cfg.CodecMode != "passthrough" {
		t.Errorf("CodecMode = %q, want %q", cfg.CodecMode, "passthrough")
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want %q", cfg.Port, "9090")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.TurnURLs != "turn:myturn.example.com:3478" {
		t.Errorf("TurnURLs = %q, want %q", cfg.TurnURLs, "turn:myturn.example.com:3478")
	}
	if cfg.TurnUsername != "turnuser" {
		t.Errorf("TurnUsername = %q, want %q", cfg.TurnUsername, "turnuser")
	}
	if cfg.TurnPassword != "turnpass" {
		t.Errorf("TurnPassword = %q, want %q", cfg.TurnPassword, "turnpass")
	}
	if cfg.RateLimitRooms != 5 {
		t.Errorf("RateLimitRooms = %d, want 5", cfg.RateLimitRooms)
	}
	if cfg.RateLimitWS != 15 {
		t.Errorf("RateLimitWS = %d, want 15", cfg.RateLimitWS)
	}
}
