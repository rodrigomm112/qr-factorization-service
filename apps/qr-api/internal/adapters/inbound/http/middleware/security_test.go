package middleware_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/middleware"
	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/problem"
)

// newApp trusts the in-memory test peer (0.0.0.0) to exercise X-Forwarded-Proto.
func newApp(handlers ...fiber.Handler) *fiber.App {
	app := fiber.New(fiber.Config{
		TrustProxy:       true,
		TrustProxyConfig: fiber.TrustProxyConfig{Proxies: []string{"0.0.0.0"}},
		ErrorHandler:     problem.ErrorHandler(slog.New(slog.DiscardHandler), nil),
	})
	for _, h := range handlers {
		app.Use(h)
	}
	app.Get("/probe", func(c fiber.Ctx) error { return c.SendString("ok") })
	app.Post("/probe", func(c fiber.Ctx) error { return c.SendString("ok") })
	return app
}

func do(t *testing.T, app *fiber.App, req *http.Request) *http.Response {
	t.Helper()
	resp, err := app.Test(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestHelmet_HSTSOnlyInProductionOverTLS(t *testing.T) {
	t.Parallel()

	secure := httptest.NewRequest(http.MethodGet, "/probe", http.NoBody)
	secure.Header.Set("X-Forwarded-Proto", "https")

	production := do(t, newApp(middleware.Helmet(true)), secure)
	hsts := production.Header.Get("Strict-Transport-Security")
	require.Contains(t, hsts, "max-age=31536000")
	require.Contains(t, hsts, "includeSubDomains")
	require.Contains(t, hsts, "preload")

	development := do(t, newApp(middleware.Helmet(false)), httptest.NewRequest(http.MethodGet, "/probe", http.NoBody))
	require.Empty(t, development.Header.Get("Strict-Transport-Security"))
}

func TestRequireJSON(t *testing.T) {
	t.Parallel()

	app := newApp(middleware.RequireJSON())

	t.Run("a request without a body is untouched", func(t *testing.T) {
		t.Parallel()
		resp := do(t, app, httptest.NewRequest(http.MethodGet, "/probe", http.NoBody))
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("accepted media types", func(t *testing.T) {
		t.Parallel()
		for _, contentType := range []string{"application/json", "application/json; charset=utf-8", "APPLICATION/JSON"} {
			resp, err := app.Test(newBodyRequest(contentType))
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode, contentType)
			require.NoError(t, resp.Body.Close())
		}
	})

	t.Run("rejected media types", func(t *testing.T) {
		t.Parallel()
		for _, contentType := range []string{"text/plain", "application/xml", ""} {
			resp, err := app.Test(newBodyRequest(contentType))
			require.NoError(t, err)
			require.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode, contentType)
			require.NoError(t, resp.Body.Close())
		}
	})
}

func TestBodyLimit(t *testing.T) {
	t.Parallel()

	app := newApp(middleware.BodyLimit(16))

	small, err := app.Test(newSizedRequest(8))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, small.StatusCode)
	require.NoError(t, small.Body.Close())

	large, err := app.Test(newSizedRequest(64))
	require.NoError(t, err)
	require.Equal(t, http.StatusRequestEntityTooLarge, large.StatusCode)
	require.NoError(t, large.Body.Close())
}

func newBodyRequest(contentType string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/probe", newReader(`{"a":1}`))
	if contentType != "" {
		req.Header.Set(fiber.HeaderContentType, contentType)
	} else {
		req.Header.Del(fiber.HeaderContentType)
	}
	return req
}

func newSizedRequest(size int) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/probe", newReader(strings.Repeat("x", size)))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	return req
}

func newReader(s string) io.Reader { return strings.NewReader(s) }
