package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/helmet"
)

// APIContentSecurityPolicy denies everything: the API serves JSON, no origin is legitimate.
const APIContentSecurityPolicy = "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// DocsContentSecurityPolicy relaxes /docs to 'self'; 'unsafe-inline' for Swagger UI styles.
const DocsContentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// hstsOneYear in seconds, with subdomains and preload eligibility.
const hstsOneYear = 31536000

// Helmet sets the security response headers; HSTS only in production, where TLS is on.
func Helmet(production bool) fiber.Handler {
	cfg := helmet.Config{
		XSSProtection:             "0",
		ContentTypeNosniff:        "nosniff",
		XFrameOptions:             "DENY",
		ReferrerPolicy:            "no-referrer",
		ContentSecurityPolicy:     APIContentSecurityPolicy,
		CrossOriginOpenerPolicy:   "same-origin",
		CrossOriginResourcePolicy: "same-origin",
		XDNSPrefetchControl:       "off",
		XPermittedCrossDomain:     "none",
		PermissionPolicy:          "geolocation=(), microphone=(), camera=()",
	}
	if production {
		cfg.HSTSMaxAge = hstsOneYear
		cfg.HSTSPreloadEnabled = true
	}
	return helmet.New(cfg)
}

// CORS builds the allow-list policy: no wildcard and no credentials; the token is a header.
func CORS(origins []string) fiber.Handler {
	return cors.New(cors.Config{
		AllowOrigins:     origins,
		AllowMethods:     []string{fiber.MethodGet, fiber.MethodPost, fiber.MethodOptions},
		AllowHeaders:     []string{fiber.HeaderContentType, fiber.HeaderAuthorization, fiber.HeaderXRequestID, fiber.HeaderAccept},
		ExposeHeaders:    []string{fiber.HeaderXRequestID, fiber.HeaderRetryAfter, headerRateLimit, headerRateLimitPolicy},
		AllowCredentials: false,
		MaxAge:           600,
	})
}
