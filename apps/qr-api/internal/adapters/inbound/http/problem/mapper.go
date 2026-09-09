package problem

import (
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/rodrigomm/proyectot/qr-api/internal/application"
)

// ContentType of every problem response.
const ContentType = "application/problem+json"

// From maps an app error to its problem: the one place the taxonomy meets HTTP statuses.
func From(err error) *Error {
	var already *Error
	if errors.As(err, &already) {
		return already
	}

	var validation *application.ErrValidation
	if errors.As(err, &validation) {
		issues := make([]Issue, 0, len(validation.Violations))
		for _, v := range validation.Violations {
			issues = append(issues, Issue{Pointer: v.Pointer, Code: string(v.Code), Message: v.Message})
		}
		return Validation(validation.Detail(), issues)
	}

	switch {
	case errors.Is(err, application.ErrDownstreamTimeout):
		return DownstreamTimeout(err)
	case errors.Is(err, application.ErrDownstreamUnavailable):
		return DownstreamUnavailable(err)
	case errors.Is(err, application.ErrUnauthorized):
		return Unauthorized(false)
	}

	// Errors raised by Fiber itself (routing, transport).
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		switch fiberErr.Code {
		case fiber.StatusNotFound:
			return NotFound()
		case fiber.StatusMethodNotAllowed:
			return MethodNotAllowed()
		case fiber.StatusRequestEntityTooLarge:
			return New(SlugPayloadTooLarge, "The request body is too large.")
		case fiber.StatusRequestHeaderFieldsTooLarge:
			// fasthttp raises this before routing; without the case it would be a 500.
			return RequestHeaderFieldsTooLarge()
		case fiber.StatusUnsupportedMediaType:
			return UnsupportedMediaType()
		case fiber.StatusTooManyRequests:
			return TooManyRequests(0)
		case fiber.StatusBadRequest:
			return MalformedJSON(err)
		}
	}
	return Internal(err)
}

// ErrorHandler is the app's only fiber.ErrorHandler; every error response is built here.
func ErrorHandler(logger *slog.Logger, now func() time.Time) fiber.ErrorHandler {
	if now == nil {
		now = time.Now
	}
	return func(c fiber.Ctx, err error) error {
		p := From(err)
		status := p.Status()

		if status >= 500 {
			logger.ErrorContext(c.Context(), "request failed",
				slog.String("requestId", c.RequestID()),
				slog.String("method", c.Method()),
				slog.String("path", c.Path()),
				slog.Int("status", status),
				slog.String("error", err.Error()))
		}

		if status == fiber.StatusUnauthorized {
			// RFC 6750 §3: the error parameter only when a bearer token was presented.
			value := `Bearer realm="proyectot"`
			if p.TokenPresented {
				value += `, error="invalid_token"`
			}
			c.Set(fiber.HeaderWWWAuthenticate, value)
		}
		if status == fiber.StatusTooManyRequests && p.RetryAfterSeconds > 0 {
			c.Set(fiber.HeaderRetryAfter, strconv.Itoa(p.RetryAfterSeconds))
		}

		doc := p.Document(c.Path(), c.RequestID(), now())
		return c.Status(status).JSON(doc, ContentType)
	}
}
