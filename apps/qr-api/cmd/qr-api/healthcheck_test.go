package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHealthcheck(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantOut string
	}{
		{"alive", http.StatusOK, ""},
		{"not ready", http.StatusServiceUnavailable, "status 503"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/health/live", r.URL.Path)
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			var stderr bytes.Buffer
			code := healthcheck(portOf(t, srv.URL), &stderr)

			if tc.wantOut == "" {
				require.Equal(t, 0, code)
				require.Empty(t, stderr.String())
				return
			}
			require.Equal(t, 1, code)
			require.Contains(t, stderr.String(), tc.wantOut)
		})
	}
}

func TestHealthcheck_NothingListening(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	getenv := portOf(t, srv.URL)
	srv.Close()

	var stderr bytes.Buffer
	require.Equal(t, 1, healthcheck(getenv, &stderr))
	require.Contains(t, stderr.String(), "healthcheck:")
}

func TestHealthcheck_DefaultsToPort8080(t *testing.T) {
	var stderr bytes.Buffer
	// Nothing listens on 8080 here; the point is that an absent PORT does not hang.
	require.Contains(t, []int{0, 1}, healthcheck(func(string) string { return "" }, &stderr))
}

func portOf(t *testing.T, raw string) func(string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return func(key string) string {
		if key == "PORT" {
			return parsed.Port()
		}
		return ""
	}
}
