package config

import (
	"log/slog"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"info":    slog.LevelInfo,
		"":        slog.LevelInfo,
	}
	for in, want := range cases {
		if got := parseLogLevel(in); got != want {
			t.Fatalf("parseLogLevel(%q)=%v want %v", in, got, want)
		}
	}
}

func TestSetupLogger(t *testing.T) {
	t.Setenv("LOG_FORMAT", "text")
	if l := SetupLogger("debug"); l == nil {
		t.Fatalf("expected logger")
	}
	t.Setenv("LOG_FORMAT", "json")
	if l := SetupLogger("info"); l == nil {
		t.Fatalf("expected logger")
	}
}
