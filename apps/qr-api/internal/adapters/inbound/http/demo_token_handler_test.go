package http_test

import (
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/platform/config"
)

func withDemoToken(enabled bool) option {
	return func(cfg *config.Config) { cfg.DemoTokenEnabled = enabled }
}

func TestDemoToken_IssuesATokenWithoutCredentials(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{}, withDemoToken(true))
	resp := h.post(t, "/api/v1/auth/demo-token", "", nil)

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "no-store", resp.Header.Get(fiber.HeaderCacheControl))

	body := decodeJSON(t, resp)
	require.Equal(t, "Bearer", body["tokenType"])
	require.Equal(t, "qr:compute stats:compute", body["scope"])

	claims, err := h.tokens.Verify(body["accessToken"].(string))
	require.NoError(t, err)
	require.Equal(t, "demo", claims.Subject)
	require.Contains(t, claims.Audience, "qr-api")
	require.Contains(t, claims.Audience, "stats-api")

	// The demo token is a first-class bearer for /qr.
	qr := h.post(t, "/api/v1/qr", `{"matrix":[[1,0],[0,1]]}`,
		map[string]string{"Authorization": "Bearer " + body["accessToken"].(string)})
	require.Equal(t, http.StatusOK, qr.StatusCode)
}

func TestDemoToken_RouteIsAbsentByDefault(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	resp := h.post(t, "/api/v1/auth/demo-token", "", nil)

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	body := assertProblem(t, resp, http.StatusNotFound, "not-found")
	require.Equal(t, "/api/v1/auth/demo-token", body["instance"])
}

func TestDemoToken_IsRateLimitedLikeTheTokenRoute(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{}, withDemoToken(true), func(cfg *config.Config) { cfg.AuthRateLimitMax = 2 })
	for range 2 {
		require.Equal(t, http.StatusOK, h.post(t, "/api/v1/auth/demo-token", "", nil).StatusCode)
	}
	resp := h.post(t, "/api/v1/auth/demo-token", "", nil)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	require.Equal(t, "no-store", resp.Header.Get(fiber.HeaderCacheControl))
	require.NotEmpty(t, resp.Header.Get("Retry-After"))
}
