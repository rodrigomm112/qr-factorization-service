package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/platform/logging"
)

func TestParseLevel(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]slog.Level{
		"debug": slog.LevelDebug, "DEBUG": slog.LevelDebug,
		"info": slog.LevelInfo, "": slog.LevelInfo, "nonsense": slog.LevelInfo,
		"warn": slog.LevelWarn, "warning": slog.LevelWarn,
		"error": slog.LevelError, " error ": slog.LevelError,
	} {
		require.Equal(t, want, logging.ParseLevel(in), "input %q", in)
	}
}

func TestNew_EmitsJSONAndHonoursTheLevel(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := logging.New(&buf, "warn")
	logger.Info("dropped")
	require.Empty(t, buf.String())

	logger.Warn("kept", slog.String("requestId", "abc"))
	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	require.Equal(t, "kept", line["msg"])
	require.Equal(t, "WARN", line["level"])
	require.Equal(t, "abc", line["requestId"])
}
