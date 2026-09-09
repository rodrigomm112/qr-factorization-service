package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// RequestID echoes a valid-UUID X-Request-ID, generates a v4 otherwise, and sets it before
// the chain so success, error and panic all carry it. Response normalization is off:
// fasthttp would serve `X-Request-Id` where the contract spells it `X-Request-ID`.
func RequestID() fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Get(fiber.HeaderXRequestID)
		if parsed, err := uuid.Parse(id); err != nil {
			id = uuid.NewString()
		} else {
			id = parsed.String()
		}
		c.Response().Header.DisableNormalizing()
		c.Set(fiber.HeaderXRequestID, id)
		return c.Next()
	}
}
