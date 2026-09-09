package statsclient_test

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/outbound/statsclient"
	"github.com/rodrigomm/proyectot/qr-api/internal/application"
)

const okBody = `{
  "requestId": "00000000-0000-4000-8000-000000000000",
  "summary": {"max":1,"min":0,"sum":6,"average":0.3333333333333333,"count":18,"anyDiagonal":true},
  "matrices": [
    {"index":0,"label":"Q","rows":3,"cols":3,"max":1,"min":0,"sum":3,"average":0.3333333333333333,"isDiagonal":true},
    {"index":1,"label":"R","rows":3,"cols":3,"max":1,"min":0,"sum":3,"average":0.3333333333333333,"isDiagonal":true}
  ],
  "meta": {"summationAlgorithm":"neumaier","diagonalTolerance":{"absolute":1e-12,"relative":1e-9},"elapsedMs":0.1}
}`

func sampleRequest() application.StatisticsRequest {
	return application.StatisticsRequest{
		Matrices: []application.LabeledMatrix{
			{Label: "Q", Values: [][]float64{{1, 0}, {0, 1}}},
			{Label: "R", Values: [][]float64{{2, 3}, {0, 4}}},
		},
		Authorization: "Bearer caller-token",
		RequestID:     "req-42",
	}
}

func newClient(baseURL string, retries int, timeout time.Duration) *statsclient.Client {
	return statsclient.New(statsclient.Config{
		BaseURL: baseURL, Timeout: timeout, Retries: retries, UserAgent: "qr-api/test",
	})
}

func TestCompute_PropagatesHeadersAndParsesTheBody(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, okBody)
	}))
	defer srv.Close()

	report, err := newClient(srv.URL, 1, time.Second).Compute(context.Background(), sampleRequest())
	require.NoError(t, err)

	require.Equal(t, http.MethodPost, captured.Method)
	require.Equal(t, "/api/v1/statistics", captured.URL.Path)
	require.Equal(t, "Bearer caller-token", captured.Header.Get("Authorization"))
	require.Equal(t, "req-42", captured.Header.Get("X-Request-ID"))
	require.Equal(t, "application/json", captured.Header.Get("Content-Type"))
	require.Equal(t, "application/json", captured.Header.Get("Accept"))
	require.Equal(t, "qr-api/test", captured.Header.Get("User-Agent"))

	var sent struct {
		Matrices []application.LabeledMatrix `json:"matrices"`
	}
	require.NoError(t, json.Unmarshal(body, &sent))
	require.Equal(t, sampleRequest().Matrices, sent.Matrices)

	require.Equal(t, 18, report.Summary.Count)
	require.True(t, report.Summary.AnyDiagonal)
	require.Len(t, report.Matrices, 2)
	require.Equal(t, "neumaier", report.Meta.SummationAlgorithm)
	require.InDelta(t, 1e-12, report.Meta.DiagonalTolerance.Absolute, 0)

	// The downstream requestId is dropped: the exchange's correlation id is qr-api's.
	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "requestId")
}

func TestCompute_RetriesOnceOn5xx(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := newClient(srv.URL, 1, time.Second).Compute(context.Background(), sampleRequest())
	require.ErrorIs(t, err, application.ErrDownstreamUnavailable)
	require.Equal(t, int32(2), attempts.Load(), "one retry means exactly two attempts")
}

func TestCompute_RecoversOnTheRetry(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = io.WriteString(w, okBody)
	}))
	defer srv.Close()

	report, err := newClient(srv.URL, 1, time.Second).Compute(context.Background(), sampleRequest())
	require.NoError(t, err)
	require.Equal(t, 18, report.Summary.Count)
	require.Equal(t, int32(2), attempts.Load())
}

func TestCompute_NeverRetriesA4xx(t *testing.T) {
	t.Parallel()

	for name, status := range map[string]int{
		"unauthorized":    http.StatusUnauthorized,
		"forbidden":       http.StatusForbidden,
		"unprocessable":   http.StatusUnprocessableEntity,
		"too many":        http.StatusTooManyRequests,
		"bad request":     http.StatusBadRequest,
		"not found":       http.StatusNotFound,
		"payload too big": http.StatusRequestEntityTooLarge,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var attempts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts.Add(1)
				w.WriteHeader(status)
			}))
			defer srv.Close()

			_, err := newClient(srv.URL, 1, time.Second).Compute(context.Background(), sampleRequest())
			require.ErrorIs(t, err, application.ErrDownstreamUnavailable)
			require.NotErrorIs(t, err, application.ErrDownstreamTimeout)
			require.Equal(t, int32(1), attempts.Load(), "a 4xx is a verdict, not a hiccup")
		})
	}
}

func TestCompute_ConnectionRefused(t *testing.T) {
	t.Parallel()

	// Bind and immediately release a port so nothing is listening on it.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())

	_, err = newClient("http://"+addr, 1, time.Second).Compute(context.Background(), sampleRequest())
	require.ErrorIs(t, err, application.ErrDownstreamUnavailable)
}

func TestCompute_SlowDependencyTimesOut(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer func() {
		close(release)
		srv.Close()
	}()

	_, err := newClient(srv.URL, 1, 100*time.Millisecond).Compute(context.Background(), sampleRequest())
	require.ErrorIs(t, err, application.ErrDownstreamTimeout)
	require.Eventually(t, func() bool { return attempts.Load() == 2 }, time.Second, 5*time.Millisecond,
		"a timeout is retried like any transient failure")
}

func TestCompute_UnparseableBody(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"not json":       "<html>oops</html>",
		"wrong shape":    `{"summary":{"max":"one"}}`,
		"empty matrices": `{"summary":{},"matrices":[],"meta":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var attempts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts.Add(1)
				_, _ = io.WriteString(w, body)
			}))
			defer srv.Close()

			_, err := newClient(srv.URL, 1, time.Second).Compute(context.Background(), sampleRequest())
			require.ErrorIs(t, err, application.ErrDownstreamUnavailable)
			require.Equal(t, int32(1), attempts.Load(), "a malformed 200 is not retried")
		})
	}
}

func TestCompute_RejectsABodyMissingAFigure(t *testing.T) {
	t.Parallel()

	// A null or absent figure would decode to a silent 0 and be echoed as a statistic. -> 502.
	for name, body := range map[string]string{
		"summary absent":       strings.Replace(okBody, `"summary"`, `"summaryX"`, 1),
		"summary null":         strings.Replace(okBody, `"summary": {`, `"summary": null, "unused": {`, 1),
		"summary.sum null":     strings.Replace(okBody, `"sum":6`, `"sum":null`, 1),
		"summary.count absent": strings.Replace(okBody, `"count":18,`, ``, 1),
		"summary.average null": strings.Replace(okBody, `"average":0.3333333333333333,"count"`, `"average":null,"count"`, 1),
		"matrix.max null":      strings.Replace(okBody, `"label":"R","rows":3,"cols":3,"max":1`, `"label":"R","rows":3,"cols":3,"max":null`, 1),
		"matrix.rows absent":   strings.Replace(okBody, `"label":"Q","rows":3,`, `"label":"Q",`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var attempts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts.Add(1)
				_, _ = io.WriteString(w, body)
			}))
			defer srv.Close()

			_, err := newClient(srv.URL, 1, time.Second).Compute(context.Background(), sampleRequest())
			require.ErrorIs(t, err, application.ErrDownstreamUnavailable)
			require.Equal(t, int32(1), attempts.Load(), "a malformed 200 is not retried")
		})
	}
}

func TestCompute_AcceptsZeroFigures(t *testing.T) {
	t.Parallel()

	body := strings.NewReplacer(`"sum":6`, `"sum":0`, `"count":18`, `"count":0`).Replace(okBody)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	report, err := newClient(srv.URL, 1, time.Second).Compute(context.Background(), sampleRequest())
	require.NoError(t, err)
	require.Zero(t, report.Summary.Sum)
	require.Zero(t, report.Summary.Count)
}

func TestCompute_DownstreamNumericalOverflowIsTheCallersFault(t *testing.T) {
	t.Parallel()

	const overflow = `{"type":"urn:proyectot:problem:validation-error","title":"The request body failed validation",` +
		`"status":422,"instance":"/api/v1/statistics","requestId":"00000000-0000-4000-8000-000000000000",` +
		`"timestamp":"2026-09-08T18:30:00.000Z","errors":[` +
		`{"pointer":"/matrices","code":"numerical_overflow","message":"the sum of the values overflows the double range"}]}`
	const otherIssue = `{"type":"urn:proyectot:problem:validation-error","status":422,` +
		`"errors":[{"pointer":"/matrices","code":"too_many_matrices","message":"nope"}]}`

	for _, tc := range []struct {
		name string
		body string
		want error
	}{
		{"numerical overflow", overflow, application.ErrDownstreamNumericalOverflow},
		{"any other 422", otherIssue, application.ErrDownstreamUnavailable},
		{"422 with no problem body", "", application.ErrDownstreamUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var attempts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts.Add(1)
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			_, err := newClient(srv.URL, 1, time.Second).Compute(context.Background(), sampleRequest())
			require.ErrorIs(t, err, tc.want)
			require.Equal(t, int32(1), attempts.Load(), "a 422 is a verdict, not a hiccup")
		})
	}
}

func TestCompute_HonoursTheCallerContext(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, okBody)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newClient(srv.URL, 1, time.Second).Compute(ctx, sampleRequest())
	require.Error(t, err)
}

// A retry that cannot finish inside what is left of the caller's deadline is not started.
func TestCompute_DoesNotRetryWithoutBudgetForAWholeAttempt(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := newClient(srv.URL, 1, 500*time.Millisecond).Compute(ctx, sampleRequest())
	require.ErrorIs(t, err, application.ErrDownstreamUnavailable)
	require.Equal(t, int32(1), attempts.Load(), "the retry would have outlived the request budget")
}

func TestCompute_RetriesWhenTheBudgetStillCoversAnAttempt(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, okBody)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	report, err := newClient(srv.URL, 1, 100*time.Millisecond).Compute(ctx, sampleRequest())
	require.NoError(t, err)
	require.Equal(t, int32(2), attempts.Load())
	require.Equal(t, 18, report.Summary.Count)
}

func TestCompute_AttemptIsCappedByTheCallerDeadline(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		_, _ = io.WriteString(w, okBody)
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := newClient(srv.URL, 1, 30*time.Second).Compute(ctx, sampleRequest())
	require.ErrorIs(t, err, application.ErrDownstreamTimeout)
	require.Less(t, time.Since(start), 5*time.Second, "the 30 s per-attempt timeout never applies past the budget")
}

func TestPing(t *testing.T) {
	t.Parallel()

	t.Run("healthy", func(t *testing.T) {
		t.Parallel()
		var path string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path = r.URL.Path
			w.Header().Set("Content-Type", "application/health+json")
			_, _ = io.WriteString(w, `{"status":"pass"}`)
		}))
		defer srv.Close()

		require.NoError(t, newClient(srv.URL, 0, time.Second).Ping(context.Background()))
		require.Equal(t, "/health/live", path)
	})

	t.Run("unhealthy status", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		err := newClient(srv.URL, 0, time.Second).Ping(context.Background())
		require.ErrorIs(t, err, application.ErrDownstreamUnavailable)
	})

	t.Run("unreachable", func(t *testing.T) {
		t.Parallel()
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := listener.Addr().String()
		require.NoError(t, listener.Close())

		err = newClient("http://"+addr, 0, time.Second).Ping(context.Background())
		require.ErrorIs(t, err, application.ErrDownstreamUnavailable)
	})
}
