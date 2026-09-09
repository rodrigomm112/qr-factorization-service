package http

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/middleware"
	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/problem"
	"github.com/rodrigomm/proyectot/qr-api/internal/application"
	"github.com/rodrigomm/proyectot/qr-api/internal/platform/config"
)

const serviceName = "qr-api"

// tokenPath is the one route with its own, stricter rate limit.
const (
	tokenPath     = "/api/v1/auth/token"      //nolint:gosec // G101 false positive: a route, not a credential
	demoTokenPath = "/api/v1/auth/demo-token" //nolint:gosec // G101 false positive: a route, not a credential
)

// bodyLimitBackstopFactor scales fasthttp's hard limit above the contractual one: 2x caps
// a request at 2 MiB, while the middleware owns the 413 above MAX_BODY_BYTES.
const bodyLimitBackstopFactor = 2

// readBufferSize: fasthttp's 4 KiB default rejects a long bearer token; above this, 431.
const readBufferSize = 16384

// Deps are the collaborators of the HTTP layer, every one an interface or a use case.
type Deps struct {
	Decompose *application.DecomposeAndAnalyze
	Tokens    *application.IssueToken
	// DemoTokens is nil when DEMO_TOKEN_ENABLED is false; the route then does not exist (404).
	DemoTokens *application.IssueDemoToken
	Verifier   middleware.TokenVerifier
	Pinger     Pinger
	Logger     *slog.Logger
	Version    string
	// Now defaults to time.Now; the tests freeze timestamps through it.
	Now func() time.Time
}

type server struct {
	deps      Deps
	cfg       *config.Config
	version   string
	startedAt time.Time
	now       func() time.Time
	readiness *readinessCache
}

func (s *server) uptimeSeconds() float64 {
	return s.now().Sub(s.startedAt).Seconds()
}

// New builds the Fiber application; it never listens, the composition root owns the socket.
func New(cfg *config.Config, deps Deps) *fiber.App { //nolint:gocritic // the dependency set is assembled once at boot
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.DiscardHandler)
	}
	if deps.Version == "" {
		deps.Version = "dev"
	}

	s := &server{
		deps:      deps,
		cfg:       cfg,
		version:   deps.Version,
		startedAt: deps.Now(),
		now:       deps.Now,
		readiness: newReadinessCache(deps.Pinger, deps.Now),
	}

	app := fiber.New(fiber.Config{
		AppName:        serviceName,
		ServerHeader:   "",
		StrictRouting:  false,
		CaseSensitive:  true,
		BodyLimit:      cfg.MaxBodyBytes * bodyLimitBackstopFactor,
		ReadBufferSize: readBufferSize,
		ReadTimeout:    10 * time.Second,
		// Ceiling of REQUEST_BUDGET: the budget expires first, so a 504 reaches the caller.
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  60 * time.Second,
		ErrorHandler: problem.ErrorHandler(deps.Logger, deps.Now),
		TrustProxy:   cfg.TrustProxy,
		// Fiber reads X-Forwarded-For only with a ProxyHeader set *and* a trusted peer; without it
		// every request behind Cloud Run shares the balancer's IP and the rate limit goes global.
		ProxyHeader: proxyHeader(cfg.TrustProxy),
		TrustProxyConfig: fiber.TrustProxyConfig{
			// Only the immediate hop is trusted (loopback, private, link-local; Cloud Run fronts on
			// 169.254.0.0/16); anything else keeps its own IP, so X-Forwarded-* cannot be spoofed.
			Loopback:  cfg.TrustProxy,
			Private:   cfg.TrustProxy,
			LinkLocal: cfg.TrustProxy,
		},
		EnableIPValidation: true,
	})

	registerMiddleware(app, s)
	registerRoutes(app, s)
	return app
}

func registerMiddleware(app *fiber.App, s *server) {
	cfg := s.cfg

	// First, so every response, a panic or transport failure included, carries X-Request-ID.
	app.Use(middleware.RequestID())
	app.Use(recover.New(recover.Config{
		// In production the trace goes to the log through the error handler, never to the client.
		EnableStackTrace: !cfg.IsProduction(),
	}))
	app.Use(middleware.AccessLog(s.deps.Logger))
	app.Use(middleware.Helmet(cfg.IsProduction()))
	app.Use(middleware.CORS(cfg.CORSAllowedOrigins))
	app.Use(middleware.BodyLimit(cfg.MaxBodyBytes))
}

func registerRoutes(app *fiber.App, s *server) {
	cfg := s.cfg

	// Health and docs sit outside /api/v1: no auth, no rate limit, per the probe contract.
	app.Get("/health/live", s.livenessHandler)
	app.Get("/health/ready", s.readinessHandler)
	app.Get("/openapi.yaml", openAPIHandler)
	app.Get("/docs", docsHandler)
	app.Get("/docs/*", docsHandler)

	windowSeconds := max(int(cfg.RateLimitWindow.Seconds()), 1)

	// The token route has its own quota; the general limiter skips it, one policy per route.
	isTokenRoute := func(c fiber.Ctx) bool { return c.Path() == tokenPath || c.Path() == demoTokenPath }

	api := app.Group("/api/v1",
		middleware.RateLimitHeaders(windowSeconds),
		newLimiter(cfg.RateLimitMax, cfg.RateLimitWindow, windowSeconds, isTokenRoute),
		middleware.RequireJSON(),
	)

	api.Post("/auth/token",
		middleware.NoStore(),
		newLimiter(cfg.AuthRateLimitMax, cfg.RateLimitWindow, windowSeconds, nil),
		newAuthHandler(s.deps.Tokens),
	)
	if s.deps.DemoTokens != nil {
		api.Post("/auth/demo-token",
			middleware.NoStore(),
			newLimiter(cfg.AuthRateLimitMax, cfg.RateLimitWindow, windowSeconds, nil),
			newDemoTokenHandler(s.deps.DemoTokens),
		)
	}
	api.Post("/qr",
		middleware.JWT(s.deps.Verifier),
		newQrHandler(s.deps.Decompose),
	)
}

// proxyHeader returns the header c.IP() reads, or "" to keep the connection's peer address.
func proxyHeader(trustProxy bool) string {
	if trustProxy {
		return fiber.HeaderXForwardedFor
	}
	return ""
}

// newLimiter builds a fixed-window limiter that renders the shared 429 document.
func newLimiter(maxRequests int, window time.Duration, windowSeconds int, next func(fiber.Ctx) bool) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:          maxRequests,
		Expiration:   window,
		Next:         next,
		LimitReached: middleware.RateLimitReached(maxRequests, windowSeconds),
	})
}
