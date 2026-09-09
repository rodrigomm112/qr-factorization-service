package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/platform/config"
)

const (
	validSecret = "a-test-secret-of-at-least-32-bytes!!"
	validDigest = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
)

func baseEnv() map[string]string {
	return map[string]string{
		"JWT_SECRET":                validSecret,
		"AUTH_CLIENT_SECRET_SHA256": validDigest,
		"STATS_API_BASE_URL":        "http://stats-api:3000",
	}
}

func load(t *testing.T, env map[string]string) (*config.Config, error) {
	t.Helper()
	return config.Load(func(key string) string { return env[key] })
}

func TestLoad_Defaults(t *testing.T) {
	t.Parallel()

	cfg, err := load(t, baseEnv())
	require.NoError(t, err)

	require.Equal(t, "development", cfg.AppEnv)
	require.False(t, cfg.IsProduction())
	require.Equal(t, "info", cfg.LogLevel)
	require.Equal(t, 8080, cfg.Port)
	require.False(t, cfg.TrustProxy)
	require.Equal(t, "qr-api", cfg.JWTIssuer)
	require.Equal(t, []string{"qr-api", "stats-api"}, cfg.JWTAudience)
	require.Equal(t, time.Hour, cfg.JWTTTL)
	require.Equal(t, "demo-client", cfg.AuthClientID)
	require.Equal(t, 5*time.Second, cfg.StatsAPITimeout)
	require.Equal(t, 1, cfg.StatsAPIRetries)
	require.Equal(t, 12*time.Second, cfg.RequestBudget)
	require.Equal(t, config.DefaultRequestBudget, cfg.RequestBudget)
	require.Equal(t, 100, cfg.MaxMatrixRows)
	require.Equal(t, 100, cfg.MaxMatrixCols)
	require.Equal(t, 1048576, cfg.MaxBodyBytes)
	require.Equal(t, 60, cfg.RateLimitMax)
	require.Equal(t, time.Minute, cfg.RateLimitWindow)
	require.Equal(t, 10, cfg.AuthRateLimitMax)
	require.Equal(t, []string{"http://localhost:8081", "http://localhost:5173"}, cfg.CORSAllowedOrigins)
	require.Equal(t, 10*time.Second, cfg.ShutdownTimeout)
}

func TestLoad_ReadsEveryKnob(t *testing.T) {
	t.Parallel()

	env := baseEnv()
	env["APP_ENV"] = "production"
	env["LOG_LEVEL"] = "debug"
	env["PORT"] = "9090"
	env["TRUST_PROXY"] = "true"
	env["JWT_ISSUER"] = "issuer"
	env["JWT_AUDIENCE"] = " qr-api , stats-api ,"
	env["JWT_TTL"] = "15m"
	env["AUTH_CLIENT_ID"] = "ci-client"
	env["STATS_API_BASE_URL"] = "https://stats.example.com/"
	env["STATS_API_TIMEOUT"] = "250ms"
	env["STATS_API_RETRIES"] = "2"
	env["REQUEST_BUDGET"] = "8s"
	env["MAX_MATRIX_ROWS"] = "10"
	env["MAX_MATRIX_COLS"] = "12"
	env["MAX_BODY_BYTES"] = "4096"
	env["RATE_LIMIT_MAX"] = "5"
	env["RATE_LIMIT_WINDOW"] = "30s"
	env["AUTH_RATE_LIMIT_MAX"] = "2"
	env["CORS_ALLOWED_ORIGINS"] = "https://app.example.com"
	env["SHUTDOWN_TIMEOUT"] = "3s"

	cfg, err := load(t, env)
	require.NoError(t, err)
	require.True(t, cfg.IsProduction())
	require.Equal(t, "debug", cfg.LogLevel)
	require.Equal(t, 9090, cfg.Port)
	require.True(t, cfg.TrustProxy)
	require.Equal(t, []string{"qr-api", "stats-api"}, cfg.JWTAudience, "blank entries are dropped")
	require.Equal(t, 15*time.Minute, cfg.JWTTTL)
	require.Equal(t, "ci-client", cfg.AuthClientID)
	require.Equal(t, "https://stats.example.com", cfg.StatsAPIBaseURL, "the trailing slash is trimmed")
	require.Equal(t, 250*time.Millisecond, cfg.StatsAPITimeout)
	require.Equal(t, 2, cfg.StatsAPIRetries)
	require.Equal(t, 8*time.Second, cfg.RequestBudget)
	require.Equal(t, 10, cfg.MaxMatrixRows)
	require.Equal(t, 12, cfg.MaxMatrixCols)
	require.Equal(t, 4096, cfg.MaxBodyBytes)
	require.Equal(t, 5, cfg.RateLimitMax)
	require.Equal(t, 30*time.Second, cfg.RateLimitWindow)
	require.Equal(t, 2, cfg.AuthRateLimitMax)
	require.Equal(t, []string{"https://app.example.com"}, cfg.CORSAllowedOrigins)
	require.Equal(t, 3*time.Second, cfg.ShutdownTimeout)
}

func TestLoad_FailFast(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(map[string]string)
		wantKey string
	}{
		{"missing JWT secret", func(e map[string]string) { delete(e, "JWT_SECRET") }, "JWT_SECRET"},
		{"short JWT secret", func(e map[string]string) { e["JWT_SECRET"] = strings.Repeat("x", 31) }, "JWT_SECRET"},
		{"missing digest", func(e map[string]string) { delete(e, "AUTH_CLIENT_SECRET_SHA256") }, "AUTH_CLIENT_SECRET_SHA256"},
		{"short digest", func(e map[string]string) { e["AUTH_CLIENT_SECRET_SHA256"] = "abc" }, "AUTH_CLIENT_SECRET_SHA256"},
		{"uppercase digest", func(e map[string]string) { e["AUTH_CLIENT_SECRET_SHA256"] = strings.ToUpper(validDigest) }, ""},
		{"non-hex digest", func(e map[string]string) { e["AUTH_CLIENT_SECRET_SHA256"] = strings.Repeat("z", 64) }, "AUTH_CLIENT_SECRET_SHA256"},
		{"missing base URL", func(e map[string]string) { delete(e, "STATS_API_BASE_URL") }, "STATS_API_BASE_URL"},
		{"relative base URL", func(e map[string]string) { e["STATS_API_BASE_URL"] = "/statistics" }, "STATS_API_BASE_URL"},
		{"schemeless base URL", func(e map[string]string) { e["STATS_API_BASE_URL"] = "stats-api:3000" }, "STATS_API_BASE_URL"},
		{"port zero", func(e map[string]string) { e["PORT"] = "0" }, "PORT"},
		{"port too high", func(e map[string]string) { e["PORT"] = "70000" }, "PORT"},
		{"port not a number", func(e map[string]string) { e["PORT"] = "http" }, "PORT"},
		{"bad duration", func(e map[string]string) { e["STATS_API_TIMEOUT"] = "5 seconds" }, "STATS_API_TIMEOUT"},
		{"negative duration", func(e map[string]string) { e["JWT_TTL"] = "-1h" }, "JWT_TTL"},
		{"bad app env", func(e map[string]string) { e["APP_ENV"] = "staging" }, "APP_ENV"},
		{"bad log level", func(e map[string]string) { e["LOG_LEVEL"] = "verbose" }, "LOG_LEVEL"},
		{"bad boolean", func(e map[string]string) { e["TRUST_PROXY"] = "yes-please" }, "TRUST_PROXY"},
		{"wildcard origin", func(e map[string]string) { e["CORS_ALLOWED_ORIGINS"] = "https://app.example.com,*" }, "CORS_ALLOWED_ORIGINS"},
		{"empty audience", func(e map[string]string) { e["JWT_AUDIENCE"] = " , " }, "JWT_AUDIENCE"},
		{"budget above the write timeout", func(e map[string]string) { e["REQUEST_BUDGET"] = "20s" }, "REQUEST_BUDGET"},
		{"budget shorter than the downstream worst case", func(e map[string]string) { e["REQUEST_BUDGET"] = "9s" }, "REQUEST_BUDGET"},
		{"budget not a duration", func(e map[string]string) { e["REQUEST_BUDGET"] = "12 seconds" }, "REQUEST_BUDGET"},
		{"budget not positive", func(e map[string]string) { e["REQUEST_BUDGET"] = "0s" }, "REQUEST_BUDGET"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := baseEnv()
			tc.mutate(env)
			cfg, err := load(t, env)

			if tc.wantKey == "" {
				require.NoError(t, err)
				require.NotNil(t, cfg)
				return
			}
			require.Error(t, err)
			require.Nil(t, cfg)
			require.Contains(t, err.Error(), tc.wantKey)
		})
	}
}

func TestLoad_ReportsEveryProblemAtOnce(t *testing.T) {
	t.Parallel()

	_, err := load(t, map[string]string{"PORT": "0"})
	require.Error(t, err)
	for _, key := range []string{"PORT", "JWT_SECRET", "AUTH_CLIENT_SECRET_SHA256", "STATS_API_BASE_URL"} {
		require.Contains(t, err.Error(), key)
	}
}

// REQUEST_BUDGET must cover STATS_API_TIMEOUT x (1 + STATS_API_RETRIES) and expire before
// the write timeout, or the caller gets a dropped connection instead of a 504.
func TestLoad_RequestBudgetBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		budget  string
		timeout string
		retries string
		wantErr string
	}{
		{"exactly the downstream worst case", "10s", "5s", "1", ""},
		{"exactly the write timeout", "15s", "5s", "1", ""},
		{"one attempt, no retry", "5s", "5s", "0", ""},
		{"a millisecond short of the worst case", "9999ms", "5s", "1", "at least"},
		{"a millisecond past the write timeout", "15001ms", "5s", "1", "at most"},
		{"three attempts do not fit", "12s", "5s", "2", "at least"},
		{"three short attempts fit", "12s", "2s", "2", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := baseEnv()
			env["REQUEST_BUDGET"] = tc.budget
			env["STATS_API_TIMEOUT"] = tc.timeout
			env["STATS_API_RETRIES"] = tc.retries

			cfg, err := load(t, env)
			if tc.wantErr == "" {
				require.NoError(t, err)
				require.Equal(t, tc.budget, cfg.RequestBudget.String(),
					"the parsed budget round-trips (%s)", tc.budget)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), "REQUEST_BUDGET")
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestWriteTimeoutIsTheBudgetCeiling(t *testing.T) {
	t.Parallel()

	require.Equal(t, 15*time.Second, config.WriteTimeout, "the value the Fiber server is built with")
	require.Less(t, config.DefaultRequestBudget, config.WriteTimeout,
		"the default budget must leave room to encode and write the answer")
}

func TestLoad_DemoTokenFlagDefaultsToOff(t *testing.T) {
	t.Parallel()

	cfg, err := load(t, baseEnv())
	require.NoError(t, err)
	require.False(t, cfg.DemoTokenEnabled)

	env := baseEnv()
	env["DEMO_TOKEN_ENABLED"] = "true"
	cfg, err = load(t, env)
	require.NoError(t, err)
	require.True(t, cfg.DemoTokenEnabled)
}
