package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	httpadapter "github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http"
	"github.com/rodrigomm/proyectot/qr-api/internal/application"
	"github.com/rodrigomm/proyectot/qr-api/internal/domain/matrix"
	"github.com/rodrigomm/proyectot/qr-api/internal/platform/config"
	"github.com/rodrigomm/proyectot/qr-api/internal/platform/security"
)

const (
	testSecret   = "a-test-secret-of-at-least-32-bytes!!"
	testClientID = "demo-client"
	testPassword = "s3cret-value"
	testOrigin   = "http://localhost:8081"
)

type stubStats struct {
	report application.StatisticsReport
	err    error
	// panicMsg makes Compute panic, to exercise the recover middleware.
	panicMsg string
	pingErr  error

	mu    sync.Mutex
	calls int
	pings int
	last  application.StatisticsRequest
}

func (s *stubStats) Compute(_ context.Context, req application.StatisticsRequest) (application.StatisticsReport, error) {
	s.mu.Lock()
	s.calls++
	s.last = req
	s.mu.Unlock()
	if s.panicMsg != "" {
		panic(s.panicMsg)
	}
	if s.err != nil {
		return application.StatisticsReport{}, s.err
	}
	return s.report, nil
}

func (s *stubStats) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *stubStats) pingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pings
}

func (s *stubStats) lastRequest() application.StatisticsRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

func (s *stubStats) Ping(context.Context) error {
	s.mu.Lock()
	s.pings++
	s.mu.Unlock()
	return s.pingErr
}

// identityReport is the golden fixture's `statistics` member for (I3, I3).
func identityReport(t *testing.T) application.StatisticsReport {
	t.Helper()
	var golden struct {
		Statistics application.StatisticsReport `json:"statistics"`
	}
	require.NoError(t, json.Unmarshal(readFixture(t, "qr.response.identity-3x3.json"), &golden))
	return golden.Statistics
}

// tallReport is statistics.response.qr-tall-3x2.json minus `requestId`, which qr-api drops.
func tallReport(t *testing.T) application.StatisticsReport {
	t.Helper()
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(readFixture(t, "statistics.response.qr-tall-3x2.json"), &envelope))
	require.Contains(t, envelope, "requestId", "the downstream fixture carries its own requestId")
	delete(envelope, "requestId")

	trimmed, err := json.Marshal(envelope)
	require.NoError(t, err)
	var report application.StatisticsReport
	require.NoError(t, json.Unmarshal(trimmed, &report))
	return report
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "..", "contracts", "examples", name))
	require.NoError(t, err, "golden fixture %s", name)
	return data
}

type harness struct {
	app    *fiber.App
	stats  *stubStats
	tokens *security.TokenService
	cfg    *config.Config
}

type option func(*config.Config)

func newHarness(t *testing.T, stats *stubStats, opts ...option) *harness {
	t.Helper()
	return newFullHarness(t, stats, nil, nil, opts...)
}

func newLoggingHarness(t *testing.T, stats *stubStats, logger *slog.Logger, opts ...option) *harness {
	t.Helper()
	return newFullHarness(t, stats, logger, nil, opts...)
}

func newClockHarness(t *testing.T, stats *stubStats, now func() time.Time, opts ...option) *harness {
	t.Helper()
	return newFullHarness(t, stats, nil, now, opts...)
}

func newFullHarness(t *testing.T, stats *stubStats, logger *slog.Logger, now func() time.Time, opts ...option) *harness {
	t.Helper()

	cfg, err := config.Load(func(key string) string {
		switch key {
		case "JWT_SECRET":
			return testSecret
		case "AUTH_CLIENT_SECRET_SHA256":
			return security.HashSecret(testPassword)
		case "STATS_API_BASE_URL":
			return "http://stats-api:3000"
		case "CORS_ALLOWED_ORIGINS":
			return testOrigin
		default:
			return ""
		}
	})
	require.NoError(t, err)
	for _, opt := range opts {
		opt(cfg)
	}

	tokens := security.NewTokenService(security.TokenServiceConfig{
		Secret:           cfg.JWTSecret,
		Issuer:           cfg.JWTIssuer,
		Audience:         cfg.JWTAudience,
		RequiredAudience: config.ServiceAudience,
		TTL:              cfg.JWTTTL,
		Scope:            config.TokenScope,
	})

	app := httpadapter.New(cfg, httpadapter.Deps{
		Decompose: application.NewDecomposeAndAnalyze(stats,
			matrix.Limits{MaxRows: cfg.MaxMatrixRows, MaxCols: cfg.MaxMatrixCols}, cfg.RequestBudget, nil),
		Tokens:     application.NewIssueToken(security.NewClientCredentials(cfg.AuthClientID, cfg.AuthClientSecretSHA256), tokens, config.TokenScope),
		DemoTokens: demoTokensFor(cfg, tokens),
		Verifier:   tokens,
		Pinger:     stats,
		Logger:     logger,
		Version:    "test",
		Now:        now,
	})
	return &harness{app: app, stats: stats, tokens: tokens, cfg: cfg}
}

// serve needs a real listener: app.Test's fake connection reports 0.0.0.0, never trusted.
func (h *harness) serve(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = h.app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true}) }()
	t.Cleanup(func() { _ = h.app.ShutdownWithTimeout(2 * time.Second) })
	return "http://" + listener.Addr().String()
}

func (h *harness) bearer(t *testing.T) string {
	t.Helper()
	token, _, _, err := h.tokens.Issue(testClientID)
	require.NoError(t, err)
	return "Bearer " + token
}

func (h *harness) do(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	resp, err := h.app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second, FailOnTimeout: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func (h *harness) post(t *testing.T, path, body string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "http://qr-api"+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	for k, v := range headers {
		if v == "" {
			req.Header.Del(k)
			continue
		}
		req.Header.Set(k, v)
	}
	return h.do(t, req)
}

func (h *harness) get(t *testing.T, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://qr-api"+path, http.NoBody)
	require.NoError(t, err)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return h.do(t, req)
}

// overSizedRequest needs a real socket: app.Test drops the response fasthttp wrote.
func (h *harness) overSizedRequest(t *testing.T, authorization, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, h.serve(t)+"/api/v1/qr", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(fiber.HeaderAuthorization, authorization)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func (h *harness) authorized(t *testing.T, path, body string) *http.Response {
	t.Helper()
	return h.post(t, path, body, map[string]string{fiber.HeaderAuthorization: h.bearer(t)})
}

func foreignToken(t *testing.T) string {
	t.Helper()
	other := security.NewTokenService(security.TokenServiceConfig{
		Secret:           []byte("a-completely-different-secret-32b!!!"),
		Issuer:           "qr-api",
		Audience:         []string{"qr-api"},
		RequiredAudience: "qr-api",
		TTL:              time.Hour,
	})
	token, _, _, err := other.Issue(testClientID)
	require.NoError(t, err)
	return token
}

func decodeJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out), "body: %s", raw)
	return out
}

func assertProblem(t *testing.T, resp *http.Response, status int, slug string) map[string]any {
	t.Helper()
	require.Equal(t, status, resp.StatusCode)
	require.Equal(t, "application/problem+json", resp.Header.Get(fiber.HeaderContentType))

	body := decodeJSON(t, resp)
	require.Equal(t, "urn:proyectot:problem:"+slug, body["type"])
	require.Equal(t, float64(status), body["status"])
	require.NotEmpty(t, body["title"])
	require.NotEmpty(t, body["instance"])
	require.Equal(t, resp.Header.Get(fiber.HeaderXRequestID), body["requestId"],
		"the body's requestId always equals the header")
	require.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`, body["timestamp"])
	return body
}

// stripVolatile removes the members problems.md §5 requires dropping before a comparison.
func stripVolatile(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			switch {
			case k == "requestId", k == "timestamp", k == "issuedAt", k == "uptimeSeconds", strings.HasSuffix(k, "Ms"):
				continue
			default:
				out[k] = stripVolatile(v)
			}
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, v := range typed {
			out[i] = stripVolatile(v)
		}
		return out
	default:
		return value
	}
}

func mustJSON(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func nullMatrixBody(rows, cols int) string {
	var buf bytes.Buffer
	buf.WriteString(`{"matrix":[`)
	for i := range rows {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteByte('[')
		for j := range cols {
			if j > 0 {
				buf.WriteByte(',')
			}
			buf.WriteString("null")
		}
		buf.WriteByte(']')
	}
	buf.WriteString(`]}`)
	return buf.String()
}

func bigMatrixBody(rows, cols int) string {
	var buf bytes.Buffer
	buf.WriteString(`{"matrix":[`)
	for i := range rows {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteByte('[')
		for j := range cols {
			if j > 0 {
				buf.WriteByte(',')
			}
			buf.WriteString("1.5")
		}
		buf.WriteByte(']')
	}
	buf.WriteString(`]}`)
	return buf.String()
}

// demoTokensFor mirrors main.go: the demo route exists only when the config enables it.
func demoTokensFor(cfg *config.Config, tokens application.TokenIssuer) *application.IssueDemoToken {
	if !cfg.DemoTokenEnabled {
		return nil
	}
	return application.NewIssueDemoToken(tokens, config.TokenScope)
}
