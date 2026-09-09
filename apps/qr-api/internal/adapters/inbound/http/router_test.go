package http_test

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/middleware"
	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/problem"
	"github.com/rodrigomm/proyectot/qr-api/internal/platform/config"
)

func TestRouting_UnknownPathIs404Problem(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	for _, path := range []string{"/nope", "/api/v1/unknown", "/api/v2/qr"} {
		body := assertProblem(t, h.get(t, path, nil), http.StatusNotFound, "not-found")
		require.Equal(t, "Resource not found", body["title"])
		require.Equal(t, path, body["instance"])
	}
}

func TestRouting_WrongMethodIs405Problem(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/qr"},
		{http.MethodDelete, "/api/v1/qr"},
		{http.MethodPut, "/api/v1/auth/token"},
		{http.MethodPost, "/health/live"},
	} {
		req, err := http.NewRequest(tc.method, "http://qr-api"+tc.path, http.NoBody)
		require.NoError(t, err)
		body := assertProblem(t, h.do(t, req), http.StatusMethodNotAllowed, "method-not-allowed")
		require.Equal(t, "Method not allowed", body["title"])
	}
}

func TestRequestID_EchoesAValidUUIDAndGeneratesOtherwise(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})

	valid := "6f1c0f1e-6b8e-4a1e-9a53-2b5f2a2c1f77"
	resp := h.get(t, "/health/live", map[string]string{fiber.HeaderXRequestID: valid})
	require.Equal(t, valid, resp.Header.Get(fiber.HeaderXRequestID))

	for _, supplied := range []string{"", "not-a-uuid", "../../etc/passwd", "6f1c0f1e6b8e4a1e9a532b5f2a2c1f77x"} {
		resp := h.get(t, "/health/live", map[string]string{fiber.HeaderXRequestID: supplied})
		got := resp.Header.Get(fiber.HeaderXRequestID)
		require.NotEqual(t, supplied, got)
		require.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, got)
	}
}

func TestRequestID_IsPresentOnEveryResponse(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{report: identityReport(t)})
	responses := []*http.Response{
		h.get(t, "/health/live", nil),
		h.get(t, "/does-not-exist", nil),
		h.get(t, "/openapi.yaml", nil),
		h.authorized(t, "/api/v1/qr", `{"matrix":[[1]]}`),
		h.post(t, "/api/v1/qr", `{"matrix":[[1]]}`, nil),
	}
	for _, resp := range responses {
		require.NotEmpty(t, resp.Header.Get(fiber.HeaderXRequestID))
	}
}

// Read off a raw socket: every HTTP client folds header names to a canonical form and would
// hide the difference. The contract and stats-api both spell it `X-Request-ID`.
func TestRequestID_HeaderNameKeepsItsContractCasing(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	address := strings.TrimPrefix(h.serve(t), "http://")

	conn, err := net.Dial("tcp", address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetDeadline(time.Now().Add(10*time.Second)))

	_, err = conn.Write([]byte("GET /health/live HTTP/1.1\r\nHost: qr-api\r\nConnection: close\r\n\r\n"))
	require.NoError(t, err)

	wire, err := io.ReadAll(bufio.NewReader(conn))
	require.NoError(t, err)

	headers, _, found := strings.Cut(string(wire), "\r\n\r\n")
	require.True(t, found, "response has no header block")
	require.Contains(t, headers, "X-Request-ID:", "the wire bytes must carry the contract casing")
	require.NotContains(t, headers, "X-Request-Id:", "fasthttp must not have canonicalized the name")
}

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	resp := h.get(t, "/health/live", nil)

	require.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	require.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"))
	require.Equal(t, "no-referrer", resp.Header.Get("Referrer-Policy"))
	require.Equal(t, middleware.APIContentSecurityPolicy, resp.Header.Get("Content-Security-Policy"))
	require.Equal(t, "same-origin", resp.Header.Get("Cross-Origin-Opener-Policy"))
	require.Empty(t, resp.Header.Get("Strict-Transport-Security"), "HSTS is meaningless without TLS in development")
	require.Empty(t, resp.Header.Get("Server"), "the server banner is not advertised")
}

func TestSecurityHeaders_HSTSInProduction(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{}, func(c *config.Config) {
		c.AppEnv = "production"
		c.TrustProxy = true
	})

	// An untrusted peer cannot make a plain request secure by sending the header.
	for _, headers := range []map[string]string{nil, {"X-Forwarded-Proto": "https"}} {
		resp := h.get(t, "/health/live", headers)
		require.Empty(t, resp.Header.Get("Strict-Transport-Security"),
			"HSTS is never sent over plain HTTP, spoofed X-Forwarded-Proto included")
		require.Equal(t, middleware.APIContentSecurityPolicy, resp.Header.Get("Content-Security-Policy"))
	}
}

func TestCORS_AllowedOrigin(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})

	req, err := http.NewRequest(http.MethodOptions, "http://qr-api/api/v1/qr", http.NoBody)
	require.NoError(t, err)
	req.Header.Set(fiber.HeaderOrigin, testOrigin)
	req.Header.Set(fiber.HeaderAccessControlRequestMethod, http.MethodPost)
	req.Header.Set(fiber.HeaderAccessControlRequestHeaders, "authorization,content-type")

	resp := h.do(t, req)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Equal(t, testOrigin, resp.Header.Get(fiber.HeaderAccessControlAllowOrigin))
	require.Contains(t, resp.Header.Get(fiber.HeaderAccessControlAllowMethods), http.MethodPost)
	require.Contains(t, resp.Header.Get(fiber.HeaderAccessControlAllowHeaders), fiber.HeaderAuthorization)
	require.Empty(t, resp.Header.Get(fiber.HeaderAccessControlAllowCredentials), "no cookies, no credentials")
}

func TestCORS_DeniedOrigin(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})

	req, err := http.NewRequest(http.MethodOptions, "http://qr-api/api/v1/qr", http.NoBody)
	require.NoError(t, err)
	req.Header.Set(fiber.HeaderOrigin, "https://evil.example.com")
	req.Header.Set(fiber.HeaderAccessControlRequestMethod, http.MethodPost)

	resp := h.do(t, req)
	require.Empty(t, resp.Header.Get(fiber.HeaderAccessControlAllowOrigin),
		"an origin outside the allow-list is never echoed back")
}

func TestCORS_SimpleRequestFromAnAllowedOrigin(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	resp := h.get(t, "/health/ready", map[string]string{fiber.HeaderOrigin: testOrigin})
	require.Equal(t, testOrigin, resp.Header.Get(fiber.HeaderAccessControlAllowOrigin))
	require.Contains(t, resp.Header.Get(fiber.HeaderAccessControlExposeHeaders), fiber.HeaderXRequestID)
}

func TestPanic_BecomesAnOpaque500(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{panicMsg: "boom: a bug in the statistics adapter"})
	body := assertProblem(t, h.authorized(t, "/api/v1/qr", `{"matrix":[[1,2],[3,4]]}`),
		http.StatusInternalServerError, "internal-error")

	require.Equal(t, "Internal server error", body["title"])
	require.Equal(t, "An unexpected error occurred. Quote the requestId when reporting it.", body["detail"])
	require.NotContains(t, body["detail"], "boom", "the panic value never reaches the client")
	require.NotContains(t, body, "errors")
	require.NotEmpty(t, body["requestId"], "the log line is correlated by requestId")
}

func TestProblemMapper_UnknownErrorIs500(t *testing.T) {
	t.Parallel()

	p := problem.From(errors.New("something nobody mapped"))
	require.Equal(t, problem.SlugInternalError, p.Slug)
	require.Equal(t, 500, p.Status())
	require.Equal(t, "An unexpected error occurred. Quote the requestId when reporting it.", p.Detail)
}

func TestProblemMapper_KnownSlugsCarryTheirExactTitle(t *testing.T) {
	t.Parallel()

	for slug, want := range map[problem.Slug]string{
		problem.SlugMalformedJSON:         "Malformed JSON body",
		problem.SlugUnauthorized:          "Authentication required",
		problem.SlugNotFound:              "Resource not found",
		problem.SlugMethodNotAllowed:      "Method not allowed",
		problem.SlugPayloadTooLarge:       "Payload too large",
		problem.SlugUnsupportedMediaType:  "Unsupported media type",
		problem.SlugValidationError:       "The request body failed validation",
		problem.SlugTooManyRequests:       "Too many requests",
		problem.SlugInternalError:         "Internal server error",
		problem.SlugDownstreamUnavailable: "Upstream dependency unavailable",
		problem.SlugDownstreamTimeout:     "Upstream dependency timed out",
	} {
		require.Equal(t, want, problem.Title(slug), "slug %s", slug)
	}
}

func TestProblemMapper_FiberErrors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		err  error
		slug problem.Slug
	}{
		{fiber.ErrNotFound, problem.SlugNotFound},
		{fiber.ErrMethodNotAllowed, problem.SlugMethodNotAllowed},
		{fiber.ErrRequestEntityTooLarge, problem.SlugPayloadTooLarge},
		{fiber.ErrUnsupportedMediaType, problem.SlugUnsupportedMediaType},
		{fiber.ErrTooManyRequests, problem.SlugTooManyRequests},
		{fiber.ErrBadRequest, problem.SlugMalformedJSON},
		{fiber.ErrTeapot, problem.SlugInternalError},
	} {
		require.Equal(t, tc.slug, problem.From(tc.err).Slug)
	}
}

func TestOpenAPIDocumentIsServed(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	resp := h.get(t, "/openapi.yaml", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/yaml; charset=utf-8", resp.Header.Get(fiber.HeaderContentType))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "openapi: 3.1.0")
	require.Contains(t, string(body), "/api/v1/qr")

	require.Contains(t, string(body), "\n  /openapi.yaml:", "the spec declares the endpoint that serves it")
	require.Contains(t, string(body), "\n  /docs:")
	require.Contains(t, string(body), "operationId: getOpenApiSpec")
	require.Contains(t, string(body), "operationId: getDocs")
}

func TestDocs(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})

	t.Run("index", func(t *testing.T) {
		t.Parallel()
		resp := h.get(t, "/docs", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Contains(t, resp.Header.Get(fiber.HeaderContentType), "text/html")
		require.Equal(t, middleware.DocsContentSecurityPolicy, resp.Header.Get("Content-Security-Policy"),
			"the docs page relaxes the CSP to 'self', never to a CDN")

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), "/docs/swagger-ui-bundle.js")
		require.Contains(t, string(body), "/docs/init.js")
		require.NotContains(t, string(body), "cdn.jsdelivr.net", "the assets are vendored, not fetched")
	})

	t.Run("assets", func(t *testing.T) {
		t.Parallel()
		for path, contentType := range map[string]string{
			"/docs/init.js":                         "text/javascript",
			"/docs/swagger-ui.css":                  "text/css",
			"/docs/swagger-ui-bundle.js":            "text/javascript",
			"/docs/swagger-ui-standalone-preset.js": "text/javascript",
			"/docs/favicon-32x32.png":               "image/png",
		} {
			resp := h.get(t, path, nil)
			require.Equal(t, http.StatusOK, resp.StatusCode, path)
			require.Contains(t, resp.Header.Get(fiber.HeaderContentType), contentType, path)
		}
	})

	t.Run("unknown asset is a 404 problem", func(t *testing.T) {
		t.Parallel()
		assertProblem(t, h.get(t, "/docs/../spec.go", nil), http.StatusNotFound, "not-found")
		assertProblem(t, h.get(t, "/docs/nothing.js", nil), http.StatusNotFound, "not-found")
	})
}
