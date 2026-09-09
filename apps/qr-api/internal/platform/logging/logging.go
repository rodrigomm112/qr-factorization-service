// Package logging builds the process-wide structured logger.
package logging

import (
	"io"
	"log/slog"
	"strings"
)

// New returns a JSON slog logger on w; an unknown level falls back to info, not a panic.
func New(w io.Writer, level string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: ParseLevel(level)}))
}

// ParseLevel maps LOG_LEVEL to an slog level.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
