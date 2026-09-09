// Package config turns the environment into a validated struct, checked once at boot.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Config is the fully validated configuration of qr-api.
type Config struct {
	AppEnv     string
	LogLevel   string
	Port       int
	TrustProxy bool

	JWTSecret   []byte
	JWTIssuer   string
	JWTAudience []string
	JWTTTL      time.Duration

	AuthClientID           string
	AuthClientSecretSHA256 string
	// DemoTokenEnabled exposes POST /api/v1/auth/demo-token (no credentials) for the public demo.
	DemoTokenEnabled bool

	StatsAPIBaseURL string
	StatsAPITimeout time.Duration
	StatsAPIRetries int

	// RequestBudget is the total deadline of one POST /api/v1/qr, downstream retries included.
	RequestBudget time.Duration

	MaxMatrixRows int
	MaxMatrixCols int
	MaxBodyBytes  int

	RateLimitMax     int
	RateLimitWindow  time.Duration
	AuthRateLimitMax int

	CORSAllowedOrigins []string
	ShutdownTimeout    time.Duration
}

// IsProduction reports the production profile: HSTS on, panic stack traces off.
func (c *Config) IsProduction() bool { return c.AppEnv == "production" }

// ServiceAudience is the `aud` value this service requires in incoming tokens.
const ServiceAudience = "qr-api"

// WriteTimeout is the server's socket write deadline and the hard ceiling of
// REQUEST_BUDGET: the budget must expire first, or the caller never reads the 504.
const WriteTimeout = 15 * time.Second

// DefaultRequestBudget leaves ~3s under WriteTimeout and covers the default 5s x 2 attempts.
const DefaultRequestBudget = 12 * time.Second

// TokenScope is granted to every issued token. It is a scope list, not a secret.
const TokenScope = "qr:compute stats:compute" //nolint:gosec // G101 false positive: a scope string

var sha256HexRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Getenv is the environment lookup, injected so the tests never touch os.Environ.
type Getenv func(string) string

// Load reads, defaults and validates the configuration.
func Load(getenv Getenv) (*Config, error) {
	l := &loader{getenv: getenv}

	cfg := &Config{
		AppEnv:     l.oneOf("APP_ENV", "development", "development", "production"),
		LogLevel:   l.oneOf("LOG_LEVEL", "info", "debug", "info", "warn", "error"),
		Port:       l.intInRange("PORT", 8080, 1, 65535),
		TrustProxy: l.boolean("TRUST_PROXY", false),

		JWTIssuer:   l.str("JWT_ISSUER", "qr-api"),
		JWTAudience: l.csv("JWT_AUDIENCE", "qr-api,stats-api"),
		JWTTTL:      l.duration("JWT_TTL", time.Hour),

		AuthClientID:     l.str("AUTH_CLIENT_ID", "demo-client"),
		DemoTokenEnabled: l.boolean("DEMO_TOKEN_ENABLED", false),

		StatsAPITimeout: l.duration("STATS_API_TIMEOUT", 5*time.Second),
		StatsAPIRetries: l.intInRange("STATS_API_RETRIES", 1, 0, 5),
		RequestBudget:   l.duration("REQUEST_BUDGET", DefaultRequestBudget),

		MaxMatrixRows: l.intInRange("MAX_MATRIX_ROWS", 100, 1, 10000),
		MaxMatrixCols: l.intInRange("MAX_MATRIX_COLS", 100, 1, 10000),
		MaxBodyBytes:  l.intInRange("MAX_BODY_BYTES", 1<<20, 1024, 1<<30),

		RateLimitMax:     l.intInRange("RATE_LIMIT_MAX", 60, 1, 1_000_000),
		RateLimitWindow:  l.duration("RATE_LIMIT_WINDOW", time.Minute),
		AuthRateLimitMax: l.intInRange("AUTH_RATE_LIMIT_MAX", 10, 1, 1_000_000),

		CORSAllowedOrigins: l.csv("CORS_ALLOWED_ORIGINS", "http://localhost:8081,http://localhost:5173"),
		ShutdownTimeout:    l.duration("SHUTDOWN_TIMEOUT", 10*time.Second),
	}

	secret := l.str("JWT_SECRET", "")
	if len(secret) < 32 {
		l.fail("JWT_SECRET", "must be at least 32 bytes long")
	}
	cfg.JWTSecret = []byte(secret)

	digest := strings.ToLower(strings.TrimSpace(l.str("AUTH_CLIENT_SECRET_SHA256", "")))
	if !sha256HexRe.MatchString(digest) {
		l.fail("AUTH_CLIENT_SECRET_SHA256", "must be exactly 64 lowercase hex characters (the SHA-256 of the client secret)")
	}
	cfg.AuthClientSecretSHA256 = digest

	cfg.StatsAPIBaseURL = strings.TrimRight(l.str("STATS_API_BASE_URL", ""), "/")
	if u, err := url.Parse(cfg.StatsAPIBaseURL); err != nil || !u.IsAbs() || u.Host == "" {
		l.fail("STATS_API_BASE_URL", "must be an absolute URL, e.g. http://stats-api:3000")
	}

	// Checked at boot; the alternative is discovering the mismatch as a 504 in production.
	if minBudget := cfg.StatsAPITimeout * time.Duration(cfg.StatsAPIRetries+1); cfg.RequestBudget < minBudget {
		l.fail("REQUEST_BUDGET", fmt.Sprintf(
			"must be at least STATS_API_TIMEOUT x (1 + STATS_API_RETRIES) = %s, otherwise the last attempt can never finish",
			minBudget))
	}
	if cfg.RequestBudget > WriteTimeout {
		l.fail("REQUEST_BUDGET", fmt.Sprintf(
			"must be at most %s, the server's write timeout", WriteTimeout))
	}

	if len(cfg.JWTAudience) == 0 {
		l.fail("JWT_AUDIENCE", "must list at least one audience")
	}
	for _, origin := range cfg.CORSAllowedOrigins {
		if origin == "*" {
			l.fail("CORS_ALLOWED_ORIGINS", "must not contain the wildcard \"*\"")
		}
	}

	if err := l.err(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// loader accumulates every problem, so a broken environment is reported in one go.
type loader struct {
	getenv Getenv
	errs   []error
}

func (l *loader) fail(key, msg string) {
	l.errs = append(l.errs, fmt.Errorf("%s %s", key, msg))
}

func (l *loader) err() error {
	if len(l.errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid configuration: %w", errors.Join(l.errs...))
}

func (l *loader) raw(key string) string { return strings.TrimSpace(l.getenv(key)) }

func (l *loader) str(key, def string) string {
	if v := l.raw(key); v != "" {
		return v
	}
	return def
}

func (l *loader) oneOf(key, def string, allowed ...string) string {
	v := l.str(key, def)
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	l.fail(key, "must be one of "+strings.Join(allowed, ", "))
	return def
}

func (l *loader) boolean(key string, def bool) bool {
	v := l.raw(key)
	if v == "" {
		return def
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		l.fail(key, "must be a boolean (true/false)")
		return def
	}
	return parsed
}

func (l *loader) intInRange(key string, def, minValue, maxValue int) int {
	v := l.raw(key)
	if v == "" {
		return def
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		l.fail(key, "must be an integer")
		return def
	}
	if parsed < minValue || parsed > maxValue {
		l.fail(key, fmt.Sprintf("must be between %d and %d", minValue, maxValue))
		return def
	}
	return parsed
}

func (l *loader) duration(key string, def time.Duration) time.Duration {
	v := l.raw(key)
	if v == "" {
		return def
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		l.fail(key, "must be a Go duration, e.g. 5s or 1m")
		return def
	}
	if parsed <= 0 {
		l.fail(key, "must be positive")
		return def
	}
	return parsed
}

func (l *loader) csv(key, def string) []string {
	raw := l.str(key, def)
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
