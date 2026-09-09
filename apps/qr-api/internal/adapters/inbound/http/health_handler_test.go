package http_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/platform/config"
)

func TestHealthLive(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	resp := h.get(t, "/health/live", nil)

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/health+json", resp.Header.Get(fiber.HeaderContentType))

	body := decodeJSON(t, resp)
	require.Equal(t, "pass", body["status"])
	require.Equal(t, "qr-api", body["service"])
	require.Equal(t, "test", body["version"])
	require.GreaterOrEqual(t, body["uptimeSeconds"], float64(0))
	require.NotContains(t, body, "checks", "liveness never touches a dependency")
	require.Zero(t, h.stats.pingCount())
}

func TestHealthReady_Pass(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	resp := h.get(t, "/health/ready", nil)

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/health+json", resp.Header.Get(fiber.HeaderContentType))

	body := decodeJSON(t, resp)
	require.Equal(t, "pass", body["status"])

	checks := body["checks"].(map[string]any)
	statsCheck := checks["stats-api"].(map[string]any)
	require.Equal(t, "pass", statsCheck["status"])
	require.Equal(t, "datastore", statsCheck["componentType"])
	require.Equal(t, "ms", statsCheck["observedUnit"])
	require.GreaterOrEqual(t, statsCheck["observedValue"], float64(0))
	require.NotContains(t, statsCheck, "output")
	require.Equal(t, 1, h.stats.pingCount())
}

func TestHealthReady_Fail(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{pingErr: errors.New("connection refused")})
	resp := h.get(t, "/health/ready", nil)

	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	require.Equal(t, "application/health+json", resp.Header.Get(fiber.HeaderContentType),
		"a failing probe is a health document, not a problem document")

	body := decodeJSON(t, resp)
	require.Equal(t, "fail", body["status"])
	statsCheck := body["checks"].(map[string]any)["stats-api"].(map[string]any)
	require.Equal(t, "fail", statsCheck["status"])
	require.Equal(t, "stats-api probe failed", statsCheck["output"],
		"the downstream error never reaches an unauthenticated endpoint")
	require.NotContains(t, statsCheck["output"], "connection refused")
	require.NotContains(t, statsCheck, "observedValue")
}

func TestHealthReady_ObservedValueUsesTheInjectedClock(t *testing.T) {
	t.Parallel()

	frozen := time.Date(2026, 9, 8, 18, 30, 0, 0, time.UTC)
	h := newClockHarness(t, &stubStats{}, func() time.Time { return frozen })

	body := decodeJSON(t, h.get(t, "/health/ready", nil))
	statsCheck := body["checks"].(map[string]any)["stats-api"].(map[string]any)
	require.InDelta(t, 0.0, statsCheck["observedValue"], 0)
	require.Equal(t, "2026-09-08T18:30:00.000Z", statsCheck["time"])
	require.InDelta(t, 0.0, body["uptimeSeconds"], 0)
}

func TestHealthReady_CachesTheProbe(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	for range 10 {
		require.Equal(t, http.StatusOK, h.get(t, "/health/ready", nil).StatusCode)
	}
	require.Equal(t, 1, h.stats.pingCount(), "the probe result is cached, so a probe storm costs one call")
}

func TestHealth_IsOutsideAuthAndTheRateLimit(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{}, func(c *config.Config) { c.RateLimitMax = 1 })
	for range 20 {
		resp := h.get(t, "/health/live", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, "no token, no rate limit on probes")
	}
}
