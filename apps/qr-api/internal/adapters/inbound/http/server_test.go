package http_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/platform/config"
)

func rateLimitRemaining(t *testing.T, resp *http.Response) string {
	t.Helper()
	header := resp.Header.Get("RateLimit")
	require.NotEmpty(t, header, "every /api/v1 response carries the quota headers")
	_, rest, ok := strings.Cut(header, ";r=")
	require.True(t, ok, "unexpected RateLimit header %q", header)
	remaining, _, _ := strings.Cut(rest, ";")
	return remaining
}

// End-to-end proof that Config.ProxyHeader is set: Fiber reads X-Forwarded-For only with a
// proxy header *and* a trusted peer, else everyone behind Cloud Run shares one bucket.
func TestServer_RateLimitBucketsByForwardedIP(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		trustProxy bool
		wantStatus int
	}{
		{"trusted proxy: each forwarded client gets its own quota", true, http.StatusOK},
		{"untrusted peer: the forwarded header is ignored", false, http.StatusTooManyRequests},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, &stubStats{report: identityReport(t)}, func(c *config.Config) {
				c.RateLimitMax = 1
				c.TrustProxy = tc.trustProxy
			})
			base := h.serve(t)
			bearer := h.bearer(t)

			post := func(forwardedFor string) *http.Response {
				req, err := http.NewRequest(http.MethodPost, base+"/api/v1/qr", strings.NewReader(`{"matrix":[[1,2],[3,4]]}`))
				require.NoError(t, err)
				req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
				req.Header.Set(fiber.HeaderAuthorization, bearer)
				req.Header.Set(fiber.HeaderXForwardedFor, forwardedFor)
				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				t.Cleanup(func() { _ = resp.Body.Close() })
				return resp
			}

			first := post("203.0.113.9")
			require.Equal(t, http.StatusOK, first.StatusCode)
			require.Equal(t, "0", rateLimitRemaining(t, first), "the quota of 1 is spent")

			require.Equal(t, http.StatusTooManyRequests, post("203.0.113.9").StatusCode,
				"the same client is always rate limited")

			require.Equal(t, tc.wantStatus, post("198.51.100.7").StatusCode)
		})
	}
}

func TestServer_OversizedHeadersAreA431Problem(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	resp := h.overSizedRequest(t, "Bearer "+strings.Repeat("x", 32<<10), `{"matrix":[[1]]}`)

	body := assertProblem(t, resp, http.StatusRequestHeaderFieldsTooLarge, "request-header-fields-too-large")
	require.Equal(t, "Request header fields too large", body["title"])
	require.NotContains(t, body, "errors")
}

func TestServer_ALongBearerTokenFitsInTheReadBuffer(t *testing.T) {
	t.Parallel()

	// ReadBufferSize is what turns a 6 KiB Authorization header from a 500 into a 431.
	h := newHarness(t, &stubStats{})
	req, err := http.NewRequest(http.MethodPost, "http://qr-api/api/v1/qr", strings.NewReader(`{"matrix":[[1]]}`))
	require.NoError(t, err)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+strings.Repeat("x", 6<<10))

	assertProblem(t, h.do(t, req), http.StatusUnauthorized, "unauthorized")
}

func TestServer_AccessLogRecordsTheFinalStatus(t *testing.T) {
	t.Parallel()

	var sink bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&sink, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := newLoggingHarness(t, &stubStats{report: identityReport(t)}, logger)

	require.Equal(t, http.StatusUnauthorized,
		h.post(t, "/api/v1/qr", `{"matrix":[[1]]}`, nil).StatusCode)
	require.Equal(t, http.StatusUnprocessableEntity,
		h.authorized(t, "/api/v1/qr", `{"matrix":[]}`).StatusCode)
	require.Equal(t, http.StatusOK,
		h.authorized(t, "/api/v1/qr", `{"matrix":[[1,2],[3,4]]}`).StatusCode)

	statuses := make([]float64, 0, 3)
	levels := make([]string, 0, 3)
	for line := range strings.Lines(sink.String()) {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["msg"] != "http request" {
			continue
		}
		statuses = append(statuses, record["status"].(float64))
		levels = append(levels, record["level"].(string))
	}
	require.Equal(t, []float64{401, 422, 200}, statuses)
	require.Equal(t, []string{"WARN", "WARN", "INFO"}, levels, "a 4xx is a warning, not an info line")
}

func TestServer_AccessLogRecordsTheAuthenticatedSubject(t *testing.T) {
	t.Parallel()

	var sink bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&sink, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := newLoggingHarness(t, &stubStats{report: identityReport(t)}, logger)

	require.Equal(t, http.StatusOK, h.authorized(t, "/api/v1/qr", `{"matrix":[[1,2],[3,4]]}`).StatusCode)
	require.Equal(t, http.StatusOK, h.get(t, "/health/live", nil).StatusCode)

	subjects := make([]any, 0, 2)
	for line := range strings.Lines(sink.String()) {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["msg"] != "http request" {
			continue
		}
		require.NotContains(t, line, h.bearer(t), "the bearer token never reaches the log")
		subjects = append(subjects, record["sub"])
	}
	require.Equal(t, []any{testClientID, nil}, subjects,
		"the authenticated request carries its subject, the probe carries no attribute at all")
}

func TestServer_BodyLimitBackstopStaysCloseToTheContractualLimit(t *testing.T) {
	t.Parallel()

	// fasthttp buffers the whole body before any middleware, so the backstop bounds one
	// request's memory: twice MAX_BODY_BYTES, not eight times. The 413 is the middleware's.
	h := newHarness(t, &stubStats{}, func(c *config.Config) { c.MaxBodyBytes = 4096 })

	overTheContract := `{"matrix":[[1]],"pad":"` + strings.Repeat("x", 5000) + `"}`
	assertProblem(t, h.authorized(t, "/api/v1/qr", overTheContract),
		http.StatusRequestEntityTooLarge, "payload-too-large")

	overTheBackstop := `{"matrix":[[1]],"pad":"` + strings.Repeat("x", 3*4096) + `"}`
	assertProblem(t, h.overSizedRequest(t, h.bearer(t), overTheBackstop),
		http.StatusRequestEntityTooLarge, "payload-too-large")
}

func TestAuthToken_RateLimited429IsNotCacheable(t *testing.T) {
	t.Parallel()

	// The limiter short-circuits before the handler, so the 429 takes the header from the route.
	h := newHarness(t, &stubStats{}, func(c *config.Config) { c.AuthRateLimitMax = 1 })
	body := `{"clientId":"demo-client","clientSecret":"wrong"}`

	first := h.post(t, "/api/v1/auth/token", body, nil)
	require.Equal(t, http.StatusUnauthorized, first.StatusCode)
	require.Equal(t, "no-store", first.Header.Get(fiber.HeaderCacheControl))

	second := h.post(t, "/api/v1/auth/token", body, nil)
	assertProblem(t, second, http.StatusTooManyRequests, "too-many-requests")
	require.Equal(t, "no-store", second.Header.Get(fiber.HeaderCacheControl))
	require.NotEmpty(t, second.Header.Get(fiber.HeaderRetryAfter))
}
