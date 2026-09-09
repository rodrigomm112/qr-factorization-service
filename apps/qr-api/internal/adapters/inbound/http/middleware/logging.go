package middleware

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/problem"
)

// AccessLog emits one structured line per request; bodies, headers and cells never logged.
func AccessLog(logger *slog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()

		// The error handler runs above this one, so log the mapper's status, not the default 200.
		status := c.Response().StatusCode()
		if err != nil {
			status = problem.From(err).Status()
		}

		level := slog.LevelInfo
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		}

		attrs := []any{
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", status),
			slog.Float64("durationMs", float64(time.Since(start).Nanoseconds())/1e6),
			slog.String("requestId", c.RequestID()),
		}
		// `sub` attributes a sequence to a client without logging the token; omitted when absent.
		if sub := Subject(c); sub != "" {
			attrs = append(attrs, slog.String("sub", sub))
		}

		logger.Log(c.Context(), level, "http request", attrs...)
		return err
	}
}
