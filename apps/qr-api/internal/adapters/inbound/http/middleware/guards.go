package middleware

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/problem"
)

// BodyLimit rejects a body over maxBytes with the shared 413. Fiber's own BodyLimit is a
// larger backstop: fasthttp enforces it by closing the connection, rendering no document.
func BodyLimit(maxBytes int) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Request().Header.ContentLength() > maxBytes || len(c.Body()) > maxBytes {
			return problem.PayloadTooLarge(maxBytes)
		}
		return c.Next()
	}
}

// RequireJSON answers 415 unless a request with a body declares application/json.
func RequireJSON() fiber.Handler {
	return func(c fiber.Ctx) error {
		if !c.HasBody() {
			return c.Next()
		}
		if !strings.EqualFold(strings.TrimSpace(c.MediaType()), fiber.MIMEApplicationJSON) {
			return problem.UnsupportedMediaType()
		}
		return c.Next()
	}
}

// NoStore sets Cache-Control: no-store before the limiter, so even its 429 carries it.
func NoStore() fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		return c.Next()
	}
}

// RateLimitReached renders the 429 with Retry-After (RFC 9110) and the draft-8 headers.
func RateLimitReached(quota, windowSeconds int) fiber.Handler {
	return func(c fiber.Ctx) error {
		reset := windowSeconds
		if header := c.GetRespHeader(fiber.HeaderRetryAfter); header != "" {
			if parsed, err := strconv.Atoi(header); err == nil && parsed > 0 {
				reset = parsed
			}
		}
		SetRateLimitHeaders(c, quota, windowSeconds, 0, reset)
		return problem.TooManyRequests(reset)
	}
}
