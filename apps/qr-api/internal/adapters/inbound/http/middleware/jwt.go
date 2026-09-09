package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/problem"
	"github.com/rodrigomm/proyectot/qr-api/internal/platform/security"
)

// TokenVerifier verifies a bearer token.
type TokenVerifier interface {
	Verify(token string) (*security.Claims, error)
}

// subjectKey holds the authenticated subject; a private type cannot collide with a package.
type subjectKey struct{}

// Subject returns the `sub` claim of the authenticated caller, if any.
func Subject(c fiber.Ctx) string {
	sub, _ := c.Locals(subjectKey{}).(string)
	return sub
}

// JWT authenticates with an HS256 bearer token; the verifier's parser fixes the algorithm
// allow-list, issuer, audience and leeway. Every failure produces the same 401 document.
func JWT(verifier TokenVerifier) fiber.Handler {
	return func(c fiber.Ctx) error {
		header := c.Get(fiber.HeaderAuthorization)
		if header == "" {
			return problem.Unauthorized(false)
		}
		scheme, token, found := strings.Cut(header, " ")
		if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
			return problem.Unauthorized(true)
		}

		claims, err := verifier.Verify(strings.TrimSpace(token))
		if err != nil {
			return problem.Wrap(problem.SlugUnauthorized, "The access token is missing, expired or invalid.", err).
				WithTokenPresented()
		}
		c.Locals(subjectKey{}, claims.Subject)
		return c.Next()
	}
}
