package http

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/dto"
	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/problem"
	"github.com/rodrigomm/proyectot/qr-api/internal/application"
)

// tokenResponse is the body of a successful POST /api/v1/auth/token.
type tokenResponse struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	ExpiresIn   int    `json:"expiresIn"`
	IssuedAt    string `json:"issuedAt"`
	Scope       string `json:"scope"`
}

func toTokenResponse(result *application.TokenResult) tokenResponse {
	return tokenResponse{
		AccessToken: result.AccessToken,
		TokenType:   result.TokenType,
		ExpiresIn:   result.ExpiresIn,
		IssuedAt:    result.IssuedAt.UTC().Format(time.RFC3339),
		Scope:       result.Scope,
	}
}

// newDemoTokenHandler serves POST /api/v1/auth/demo-token; the body, if any, is ignored.
func newDemoTokenHandler(uc *application.IssueDemoToken) fiber.Handler {
	return func(c fiber.Ctx) error {
		result, err := uc.Execute()
		if err != nil {
			return err
		}
		return c.Status(fiber.StatusOK).JSON(toTokenResponse(result))
	}
}

func newAuthHandler(uc *application.IssueToken) fiber.Handler {
	return func(c fiber.Ctx) error {
		// Cache-Control: no-store comes from middleware.NoStore, which also covers the 429.
		req, violations, err := dto.DecodeTokenRequest(c.Body())
		if err != nil {
			return problem.MalformedJSON(err)
		}
		if len(violations) > 0 {
			return application.NewValidationError(violations)
		}

		result, err := uc.Execute(application.TokenCommand{ClientID: req.ClientID, ClientSecret: req.ClientSecret})
		if err != nil {
			if errors.Is(err, application.ErrUnauthorized) {
				// Client credentials, no bearer token: RFC 6750 §3 keeps error="invalid_token" out.
				return problem.Unauthorized(false)
			}
			return err
		}

		return c.Status(fiber.StatusOK).JSON(toTokenResponse(result))
	}
}
