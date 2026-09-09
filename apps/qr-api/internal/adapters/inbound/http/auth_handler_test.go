package http_test

import (
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/platform/config"
)

func TestAuthToken_IssuesAUsableToken(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	resp := h.post(t, "/api/v1/auth/token", `{"clientId":"demo-client","clientSecret":"s3cret-value"}`, nil)

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "no-store", resp.Header.Get(fiber.HeaderCacheControl))
	require.NotEmpty(t, resp.Header.Get(fiber.HeaderXRequestID))

	body := decodeJSON(t, resp)
	require.Equal(t, "Bearer", body["tokenType"])
	require.Equal(t, float64(3600), body["expiresIn"])
	require.Equal(t, "qr:compute stats:compute", body["scope"])
	require.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`, body["issuedAt"])

	claims, err := h.tokens.Verify(body["accessToken"].(string))
	require.NoError(t, err)
	require.Equal(t, "demo-client", claims.Subject)
	require.Equal(t, "qr-api", claims.Issuer)
	require.Contains(t, claims.Audience, "qr-api")
	require.Contains(t, claims.Audience, "stats-api")
	require.NotEmpty(t, claims.ID)
}

func TestAuthToken_RejectsBadCredentialsIdentically(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	first := mustUnauthorized(t, h, `{"clientId":"unknown","clientSecret":"s3cret-value"}`)
	second := mustUnauthorized(t, h, `{"clientId":"demo-client","clientSecret":"wrong"}`)
	require.Equal(t, stripVolatile(first), stripVolatile(second),
		"an unknown client and a wrong secret must be indistinguishable")
}

func mustUnauthorized(t *testing.T, h *harness, body string) map[string]any {
	t.Helper()
	resp := h.post(t, "/api/v1/auth/token", body, nil)
	document := assertProblem(t, resp, http.StatusUnauthorized, "unauthorized")
	require.Equal(t, "no-store", resp.Header.Get(fiber.HeaderCacheControl), "a failed exchange is not cacheable either")
	// RFC 6750 §3: no token was presented, so the challenge is the bare realm.
	require.Equal(t, `Bearer realm="proyectot"`, resp.Header.Get(fiber.HeaderWWWAuthenticate))
	return document
}

func TestAuthToken_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		pointers []string
	}{
		{"both missing", `{}`, []string{"/clientId", "/clientSecret"}},
		{"secret missing", `{"clientId":"demo-client"}`, []string{"/clientSecret"}},
		{"empty strings", `{"clientId":"","clientSecret":""}`, []string{"/clientId", "/clientSecret"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, &stubStats{})
			resp := h.post(t, "/api/v1/auth/token", tc.body, nil)
			body := assertProblem(t, resp, http.StatusUnprocessableEntity, "validation-error")

			issues, ok := body["errors"].([]any)
			require.True(t, ok)
			require.Len(t, issues, len(tc.pointers))
			for i, pointer := range tc.pointers {
				issue := issues[i].(map[string]any)
				require.Equal(t, pointer, issue["pointer"])
				require.Equal(t, "missing_field", issue["code"])
			}
		})
	}
}

func TestAuthToken_WrongFieldTypeIs422(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	body := assertProblem(t,
		h.post(t, "/api/v1/auth/token", `{"clientId":42,"clientSecret":"x"}`, nil),
		http.StatusUnprocessableEntity, "validation-error")

	issues := body["errors"].([]any)
	require.Len(t, issues, 1)
	require.Equal(t, "/clientId", issues[0].(map[string]any)["pointer"])
	require.Equal(t, "invalid_type", issues[0].(map[string]any)["code"])
}

func TestAuthToken_MalformedJSON(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{})
	for _, body := range []string{`{`, `{"clientId":`} {
		assertProblem(t, h.post(t, "/api/v1/auth/token", body, nil), http.StatusBadRequest, "malformed-json")
	}
}

// Valid JSON of the wrong root type is a 422 with the empty pointer, never a 400.
func TestAuthToken_RootBodyOfTheWrongTypeIs422(t *testing.T) {
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
			doc := assertProblem(t, h.post(t, "/api/v1/auth/token", body, nil),
				http.StatusUnprocessableEntity, "validation-error")

			issues := doc["errors"].([]any)
			require.Len(t, issues, 1)
			require.Equal(t, map[string]any{
				"pointer": "",
				"code":    "invalid_type",
				"message": "the request body must be a JSON object",
			}, issues[0])
		})
	}
}

func TestAuthToken_StricterRateLimit(t *testing.T) {
	t.Parallel()

	h := newHarness(t, &stubStats{}, func(c *config.Config) { c.AuthRateLimitMax = 3 })

	for range 3 {
		resp := h.post(t, "/api/v1/auth/token", `{"clientId":"demo-client","clientSecret":"s3cret-value"}`, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}
	resp := h.post(t, "/api/v1/auth/token", `{"clientId":"demo-client","clientSecret":"s3cret-value"}`, nil)
	assertProblem(t, resp, http.StatusTooManyRequests, "too-many-requests")
	require.Equal(t, "60", resp.Header.Get(fiber.HeaderRetryAfter))
	require.Equal(t, `"default";q=3;w=60`, resp.Header.Get("RateLimit-Policy"))
}
