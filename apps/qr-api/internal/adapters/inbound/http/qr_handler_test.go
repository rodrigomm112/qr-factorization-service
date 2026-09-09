package http_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/application"
	"github.com/rodrigomm/proyectot/qr-api/internal/platform/config"
)

func TestQr_IdentityMatchesTheGoldenFixture(t *testing.T) {
	t.Parallel()

	stats := &stubStats{report: identityReport(t)}
	h := newHarness(t, stats)

	req := readFixture(t, "qr.request.identity-3x3.json")
	resp := h.authorized(t, "/api/v1/qr", string(req))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 1, stats.calls)

	got := decodeJSON(t, resp)
	require.Equal(t, got["requestId"], resp.Header.Get(fiber.HeaderXRequestID))

	want := mustJSON(t, readFixture(t, "qr.response.identity-3x3.json"))
	require.Equal(t, stripVolatile(want), stripVolatile(got),
		"the whole document must match the golden fixture once volatile members are stripped")

	statistics := got["statistics"].(map[string]any)
	require.True(t, statistics["summary"].(map[string]any)["anyDiagonal"].(bool))
	require.NotContains(t, statistics, "requestId", "the downstream requestId is dropped")
}

func TestQr_TallMatrixShapesAndMeta(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{report: identityReport(t)})
	resp := h.authorized(t, "/api/v1/qr", string(readFixture(t, "qr.request.tall-3x2.json")))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body := decodeJSON(t, resp)
	require.Equal(t, map[string]any{"rows": float64(3), "cols": float64(2)}, body["input"])
	require.Len(t, body["q"], 3)
	require.Len(t, body["q"].([]any)[0], 3)
	require.Len(t, body["r"], 3)
	require.Len(t, body["r"].([]any)[0], 2)

	meta := body["meta"].(map[string]any)
	require.Equal(t, "householder", meta["algorithm"])
	require.Equal(t, "full", meta["mode"])
	require.Equal(t, []any{float64(3), float64(3)}, meta["qShape"])
	require.Equal(t, []any{float64(3), float64(2)}, meta["rShape"])
	require.GreaterOrEqual(t, meta["decompositionMs"], float64(0))
	require.GreaterOrEqual(t, meta["statisticsMs"], float64(0))
}

func TestQr_ReducedMode(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{report: identityReport(t)})
	body := decodeJSON(t, h.authorized(t, "/api/v1/qr", `{"matrix":[[1,2],[3,4],[5,6]],"mode":"reduced"}`))

	meta := body["meta"].(map[string]any)
	require.Equal(t, "reduced", meta["mode"])
	require.Equal(t, []any{float64(3), float64(2)}, meta["qShape"])
	require.Equal(t, []any{float64(2), float64(2)}, meta["rShape"])
}

func TestQr_WideMatrix(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{report: identityReport(t)})
	body := decodeJSON(t, h.authorized(t, "/api/v1/qr", string(readFixture(t, "qr.request.wide-2x3.json"))))
	meta := body["meta"].(map[string]any)
	require.Equal(t, []any{float64(2), float64(2)}, meta["qShape"])
	require.Equal(t, []any{float64(2), float64(3)}, meta["rShape"])
}

func TestQr_ForwardsTheCallerToken(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{report: identityReport(t)})
	authorization := h.bearer(t)

	req, err := http.NewRequest(http.MethodPost, "http://qr-api/api/v1/qr", strings.NewReader(`{"matrix":[[1,2],[3,4]]}`))
	require.NoError(t, err)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(fiber.HeaderAuthorization, authorization)
	req.Header.Set(fiber.HeaderXRequestID, "9f1b7d64-0f6e-4f2a-9f47-2f6a1a4e5c31")

	resp := h.do(t, req)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "9f1b7d64-0f6e-4f2a-9f47-2f6a1a4e5c31", resp.Header.Get(fiber.HeaderXRequestID))

	captured := h.stats.lastRequest()
	require.Equal(t, authorization, captured.Authorization, "the caller's token is forwarded verbatim")
	require.Equal(t, "9f1b7d64-0f6e-4f2a-9f47-2f6a1a4e5c31", captured.RequestID)
	require.Len(t, captured.Matrices, 2)
	require.Equal(t, "Q", captured.Matrices[0].Label)
	require.Equal(t, "R", captured.Matrices[1].Label)
}

func TestQr_Unauthorized(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	golden := stripVolatile(mustJSON(t, readFixture(t, "problem.unauthorized.json")))

	for _, tc := range []struct {
		name          string
		authorization string
		wantChallenge string
	}{
		{"missing header", "", `Bearer realm="proyectot"`},
		{"not a bearer scheme", "Basic dXNlcjpwYXNz", `Bearer realm="proyectot", error="invalid_token"`},
		{"empty bearer", "Bearer ", `Bearer realm="proyectot", error="invalid_token"`},
		{"garbage token", "Bearer not-a-jwt", `Bearer realm="proyectot", error="invalid_token"`},
		{"token signed by someone else", "Bearer " + foreignToken(t), `Bearer realm="proyectot", error="invalid_token"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp := h.post(t, "/api/v1/qr", `{"matrix":[[1]]}`,
				map[string]string{fiber.HeaderAuthorization: tc.authorization})
			body := assertProblem(t, resp, http.StatusUnauthorized, "unauthorized")
			require.Equal(t, tc.wantChallenge, resp.Header.Get(fiber.HeaderWWWAuthenticate))
			require.Equal(t, golden, stripVolatile(body))
			require.NotContains(t, body, "errors")
		})
	}
	require.Zero(t, h.stats.callCount(), "an unauthenticated request never reaches stats-api")
}

func TestQr_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		pointer string
		code    string
	}{
		{"ragged row", string(readFixture(t, "qr.request.invalid-ragged.json")), "/matrix/1", "ragged_row"},
		{"empty matrix", `{"matrix":[]}`, "/matrix", "empty_matrix"},
		{"empty row", `{"matrix":[[]]}`, "/matrix/0", "empty_row"},
		{"matrix missing", `{"mode":"full"}`, "/matrix", "missing_field"},
		{"null cell", `{"matrix":[[1,null]]}`, "/matrix/0/1", "invalid_type"},
		{"string cell", `{"matrix":[["1"]]}`, "/matrix/0/0", "invalid_type"},
		{"boolean cell", `{"matrix":[[true]]}`, "/matrix/0/0", "invalid_type"},
		{"object cell", `{"matrix":[[{}]]}`, "/matrix/0/0", "invalid_type"},
		{"overflowing literal", `{"matrix":[[1e999]]}`, "/matrix/0/0", "non_finite_value"},
		{"unknown mode", `{"matrix":[[1]],"mode":"thin"}`, "/mode", "invalid_mode"},
		{"101 rows", bigMatrixBody(101, 1), "/matrix", "too_many_rows"},
		{"101 columns", bigMatrixBody(1, 101), "/matrix", "too_many_cols"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, &stubStats{})
			body := assertProblem(t, h.authorized(t, "/api/v1/qr", tc.body), http.StatusUnprocessableEntity, "validation-error")

			issues := body["errors"].([]any)
			require.NotEmpty(t, issues)
			first := issues[0].(map[string]any)
			require.Equal(t, tc.pointer, first["pointer"])
			require.Equal(t, tc.code, first["code"])
			require.Contains(t, body["detail"], first["message"])
			require.Zero(t, h.stats.callCount(), "a rejected request never reaches stats-api")
		})
	}
}

func TestQr_RaggedRowMatchesTheGoldenProblem(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	// The golden document quotes "row 1 has 3 columns, expected 2".
	body := assertProblem(t,
		h.authorized(t, "/api/v1/qr", `{"matrix":[[1,2],[1,2,3]]}`),
		http.StatusUnprocessableEntity, "validation-error")

	want := stripVolatile(mustJSON(t, readFixture(t, "problem.validation-error.json")))
	require.Equal(t, want, stripVolatile(body))
}

func TestQr_RaggedFixtureRequest(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	body := assertProblem(t,
		h.authorized(t, "/api/v1/qr", string(readFixture(t, "qr.request.invalid-ragged.json"))),
		http.StatusUnprocessableEntity, "validation-error")

	require.Equal(t, "matrix must be rectangular: row 1 has 3 columns, expected 2", body["detail"])
	issues := body["errors"].([]any)
	require.Len(t, issues, 1)
	require.Equal(t, "/matrix/1", issues[0].(map[string]any)["pointer"])
	require.Equal(t, "ragged_row", issues[0].(map[string]any)["code"])
}

func TestQr_ReportsEveryFailureAtOnce(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	body := assertProblem(t,
		h.authorized(t, "/api/v1/qr", `{"matrix":[[1,null],[2,3,4]],"mode":"thin"}`),
		http.StatusUnprocessableEntity, "validation-error")

	issues := body["errors"].([]any)
	pointers := make([]string, 0, len(issues))
	for _, issue := range issues {
		pointers = append(pointers, issue.(map[string]any)["pointer"].(string))
	}
	require.Equal(t, []string{"/matrix/0/1", "/mode"}, pointers, "ordered by pointer")
	require.Equal(t, "The request body contains 2 validation errors.", body["detail"],
		"several failures are summarized by their count, never by the first message")
}

// Decoding runs first and reports every unreadable cell; the shape rules only see a body
// that decoded cleanly, so a row that lost a cell has no width to compare.
func TestQr_DecodingAndValidationAreTwoPhases(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	body := assertProblem(t,
		h.authorized(t, "/api/v1/qr", `{"matrix":[[1,null],[3,4,5]]}`),
		http.StatusUnprocessableEntity, "validation-error")

	issues := body["errors"].([]any)
	require.Len(t, issues, 1, "the ragged second row is not reported alongside the undecodable cell")
	require.Equal(t, "/matrix/0/1", issues[0].(map[string]any)["pointer"])
	require.Equal(t, "invalid_type", issues[0].(map[string]any)["code"])
	require.Equal(t, "cell [0][1] must be a number", body["detail"],
		"a single failure renders its own message")
	require.Zero(t, h.stats.callCount())
}

func TestQr_ValidationErrorsAreCappedAt200(t *testing.T) {
	t.Parallel()

	// ~200 000 failing rules: `errors[]` is capped at 200 and `detail` states the real total.
	h := newHarness(t, &stubStats{})
	body := nullMatrixBody(100, 2000)
	require.Less(t, len(body), h.cfg.MaxBodyBytes)
	require.Greater(t, len(body), h.cfg.MaxBodyBytes*9/10, "the body must be close to the 1 MiB limit")

	resp := h.authorized(t, "/api/v1/qr", body)
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	require.Less(t, len(raw), 100<<10, "the problem document stays orders of magnitude below the request")

	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	issues := doc["errors"].([]any)
	require.Len(t, issues, application.MaxReportedViolations)
	require.Equal(t, "/matrix/0/0", issues[0].(map[string]any)["pointer"], "the lowest pointers survive")
	require.Equal(t,
		"The request body contains 200000 validation errors; the first 200 are listed.",
		doc["detail"])
	require.Zero(t, h.stats.callCount())
}

func TestQr_DownstreamFailures(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		err    error
		status int
		slug   string
	}{
		{"unavailable", application.ErrDownstreamUnavailable, http.StatusBadGateway, "downstream-unavailable"},
		{"timeout", application.ErrDownstreamTimeout, http.StatusGatewayTimeout, "downstream-timeout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, &stubStats{err: tc.err})
			body := assertProblem(t, h.authorized(t, "/api/v1/qr", `{"matrix":[[1,2],[3,4]]}`), tc.status, tc.slug)
			require.NotContains(t, body, "errors")
		})
	}
}

func TestQr_DownstreamNumericalOverflowBecomesOur422(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{err: application.ErrDownstreamNumericalOverflow})
	body := assertProblem(t, h.authorized(t, "/api/v1/qr", `{"matrix":[[1,2],[3,4]]}`),
		http.StatusUnprocessableEntity, "validation-error")

	issues := body["errors"].([]any)
	require.Len(t, issues, 1)
	require.Equal(t, "/matrix", issues[0].(map[string]any)["pointer"])
	require.Equal(t, "numerical_overflow", issues[0].(map[string]any)["code"])
}

func TestQr_DownstreamUnavailableMatchesTheGoldenProblem(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{err: application.ErrDownstreamUnavailable})
	body := assertProblem(t, h.authorized(t, "/api/v1/qr", `{"matrix":[[1,2],[3,4],[5,6]]}`),
		http.StatusBadGateway, "downstream-unavailable")
	require.Equal(t, stripVolatile(mustJSON(t, readFixture(t, "problem.downstream-unavailable.json"))), stripVolatile(body))
}

func TestQr_MalformedJSON(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	for _, body := range []string{`{`, `{"matrix":[[1,2]`, `{"matrix":[[NaN]]}`, `{"matrix":[[Infinity]]}`} {
		assertProblem(t, h.authorized(t, "/api/v1/qr", body), http.StatusBadRequest, "malformed-json")
	}
}

// A syntax error is 400; a wrong root type is 422 with pointer "" (problems.md §3).
func TestQr_RootBodyOfTheWrongTypeIs422(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"array":  `[1,2,3]`,
		"string": `"x"`,
		"number": `123`,
		"null":   `null`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, &stubStats{})
			doc := assertProblem(t, h.authorized(t, "/api/v1/qr", body),
				http.StatusUnprocessableEntity, "validation-error")

			issues := doc["errors"].([]any)
			require.Len(t, issues, 1)
			require.Equal(t, map[string]any{
				"pointer": "",
				"code":    "invalid_type",
				"message": "the request body must be a JSON object",
			}, issues[0])
			require.Equal(t, "the request body must be a JSON object", doc["detail"])
			require.Zero(t, h.stats.callCount(), "a rejected request never reaches stats-api")
		})
	}
}

// The shared 400 envelope, emitted byte for byte except `instance`.
func TestQr_MalformedJSONMatchesTheGoldenProblem(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	doc := assertProblem(t, h.authorized(t, "/api/v1/qr", `{`), http.StatusBadRequest, "malformed-json")

	want := stripVolatile(mustJSON(t, readFixture(t, "problem.malformed-json.json"))).(map[string]any)
	want["instance"] = "/api/v1/qr"
	require.Equal(t, want, stripVolatile(doc))
}

func TestQr_UnsupportedMediaType(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	for _, contentType := range []string{"text/plain", "application/xml", "application/x-www-form-urlencoded", ""} {
		resp := h.post(t, "/api/v1/qr", `{"matrix":[[1]]}`, map[string]string{
			fiber.HeaderAuthorization: h.bearer(t),
			fiber.HeaderContentType:   contentType,
		})
		assertProblem(t, resp, http.StatusUnsupportedMediaType, "unsupported-media-type")
	}
}

func TestQr_AcceptsAJSONCharsetParameter(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{report: identityReport(t)})
	resp := h.post(t, "/api/v1/qr", `{"matrix":[[1]]}`, map[string]string{
		fiber.HeaderAuthorization: h.bearer(t),
		fiber.HeaderContentType:   "application/json; charset=utf-8",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestQr_PayloadTooLarge(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{}, func(c *config.Config) { c.MaxBodyBytes = 2048 })
	body := bigMatrixBody(40, 20)
	require.Greater(t, len(body), h.cfg.MaxBodyBytes)
	require.Less(t, len(body), h.cfg.MaxBodyBytes*2)

	assertProblem(t, h.authorized(t, "/api/v1/qr", body), http.StatusRequestEntityTooLarge, "payload-too-large")
	require.Zero(t, h.stats.callCount())
}

func TestQr_RateLimit(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{report: identityReport(t)}, func(c *config.Config) { c.RateLimitMax = 2 })
	authorization := h.bearer(t)
	for range 2 {
		resp := h.post(t, "/api/v1/qr", `{"matrix":[[1]]}`, map[string]string{fiber.HeaderAuthorization: authorization})
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}
	resp := h.post(t, "/api/v1/qr", `{"matrix":[[1]]}`, map[string]string{fiber.HeaderAuthorization: authorization})
	assertProblem(t, resp, http.StatusTooManyRequests, "too-many-requests")
	require.Equal(t, "60", resp.Header.Get(fiber.HeaderRetryAfter))
	require.Equal(t, `"default";q=2;w=60`, resp.Header.Get("RateLimit-Policy"))
	require.Regexp(t, `^"default";r=0;t=\d+$`, resp.Header.Get("RateLimit"),
		"the rejected request reports an exhausted quota, not a missing one")
}

func TestQr_TallMatrixMatchesTheGeneratedFixtures(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{report: tallReport(t)})
	resp := h.authorized(t, "/api/v1/qr", string(readFixture(t, "qr.request.tall-3x2.json")))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Frozen in contracts/examples: a digit that moves fails here instead of drifting.
	body := decodeJSON(t, resp)
	want := mustJSON(t, readFixture(t, "qr.output.tall-3x2.json"))
	require.Equal(t, want["q"], body["q"])
	require.Equal(t, want["r"], body["r"])

	golden := mustJSON(t, readFixture(t, "qr.response.tall-3x2.json"))
	require.Equal(t, golden["q"], body["q"])
	require.Equal(t, golden["r"], body["r"])
	require.Equal(t, golden["input"], body["input"])

	goldenMeta := golden["meta"].(map[string]any)
	meta := body["meta"].(map[string]any)
	for _, member := range []string{"algorithm", "mode", "qShape", "rShape"} {
		require.Equal(t, goldenMeta[member], meta[member], "meta.%s", member)
	}
	require.Equal(t, stripVolatile(golden), stripVolatile(body),
		"the whole document must match the golden fixture once volatile members are stripped")

	var sent map[string]any
	encoded, err := json.Marshal(map[string]any{"matrices": h.stats.lastRequest().Matrices})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &sent))
	require.Equal(t, mustJSON(t, readFixture(t, "statistics.request.qr-tall-3x2.json")), sent)
}

func TestQr_RateLimitHeadersAreDraft8(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{report: identityReport(t)}, func(c *config.Config) { c.RateLimitMax = 5 })
	resp := h.authorized(t, "/api/v1/qr", `{"matrix":[[1]]}`)

	require.Equal(t, `"default";q=5;w=60`, resp.Header.Get("RateLimit-Policy"))
	require.Regexp(t, `^"default";r=4;t=\d+$`, resp.Header.Get("RateLimit"))
	require.Empty(t, resp.Header.Get("X-RateLimit-Limit"), "the pre-standard trio is not emitted")
	require.Empty(t, resp.Header.Get("X-RateLimit-Remaining"))
	require.Empty(t, resp.Header.Get("X-RateLimit-Reset"))
}

func TestAuthToken_RateLimitPolicyIsTheStricterOne(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{}, func(c *config.Config) { c.AuthRateLimitMax = 4 })
	resp := h.post(t, "/api/v1/auth/token", `{"clientId":"demo-client","clientSecret":"s3cret-value"}`, nil)
	require.Equal(t, `"default";q=4;w=60`, resp.Header.Get("RateLimit-Policy"))
}
