// Package statsclient is the outbound adapter to stats-api: retries, deadlines, 502/504.
package statsclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/rodrigomm/proyectot/qr-api/internal/application"
	"github.com/rodrigomm/proyectot/qr-api/internal/domain/matrix"
)

// maxResponseBytes caps the read so a misbehaving upstream cannot exhaust the heap.
const maxResponseBytes = 8 << 20

// Config configures the client.
type Config struct {
	BaseURL string
	// Timeout is per attempt; the caller's context (REQUEST_BUDGET) bounds the total.
	Timeout time.Duration
	// Retries is the number of extra idempotent attempts (1 => at most 2).
	Retries   int
	UserAgent string
	Logger    *slog.Logger
}

// Client talks to stats-api over net/http rather than Fiber's client: the outbound adapter
// keeps its own transport and per-attempt deadlines, independent of the server framework.
type Client struct {
	cfg  Config
	http *http.Client
}

// New builds the client; the http.Client has no global timeout, ctx sets one per attempt.
func New(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 32
	return &Client{cfg: cfg, http: &http.Client{Transport: transport}}
}

var _ application.StatisticsClient = (*Client)(nil)

// wireReport is the stats-api body minus `requestId`. Every figure decodes through a pointer,
// so an absent or null member is told apart from a legitimate 0 rather than echoed as one.
type wireReport struct {
	Summary  *wireSummary               `json:"summary"`
	Matrices []wireMatrix               `json:"matrices"`
	Meta     application.StatisticsMeta `json:"meta"`
}

type wireSummary struct {
	Max         *float64 `json:"max"`
	Min         *float64 `json:"min"`
	Sum         *float64 `json:"sum"`
	Average     *float64 `json:"average"`
	Count       *int     `json:"count"`
	AnyDiagonal bool     `json:"anyDiagonal"`
}

type wireMatrix struct {
	Index      *int     `json:"index"`
	Label      string   `json:"label"`
	Rows       *int     `json:"rows"`
	Cols       *int     `json:"cols"`
	Max        *float64 `json:"max"`
	Min        *float64 `json:"min"`
	Sum        *float64 `json:"sum"`
	Average    *float64 `json:"average"`
	IsDiagonal bool     `json:"isDiagonal"`
}

// report validates and flattens the decoded body; a missing figure is malformed, not 0.
func (w *wireReport) report() (application.StatisticsReport, error) {
	if w.Summary == nil {
		return application.StatisticsReport{}, missingField("summary")
	}
	if len(w.Matrices) == 0 {
		return application.StatisticsReport{}, fmt.Errorf("%w: response carries no matrices", application.ErrDownstreamUnavailable)
	}
	sum := w.Summary
	if sum.Max == nil || sum.Min == nil || sum.Sum == nil || sum.Average == nil || sum.Count == nil {
		return application.StatisticsReport{}, missingField("summary")
	}

	matrices := make([]application.MatrixStatistics, len(w.Matrices))
	for i := range w.Matrices {
		m := &w.Matrices[i]
		if m.Index == nil || m.Rows == nil || m.Cols == nil ||
			m.Max == nil || m.Min == nil || m.Sum == nil || m.Average == nil {
			return application.StatisticsReport{}, missingField(fmt.Sprintf("matrices[%d]", i))
		}
		matrices[i] = application.MatrixStatistics{
			Index: *m.Index, Label: m.Label, Rows: *m.Rows, Cols: *m.Cols,
			Max: *m.Max, Min: *m.Min, Sum: *m.Sum, Average: *m.Average,
			IsDiagonal: m.IsDiagonal,
		}
	}

	return application.StatisticsReport{
		Summary: application.StatisticsSummary{
			Max: *sum.Max, Min: *sum.Min, Sum: *sum.Sum, Average: *sum.Average,
			Count: *sum.Count, AnyDiagonal: sum.AnyDiagonal,
		},
		Matrices: matrices,
		Meta:     w.Meta,
	}, nil
}

func missingField(where string) error {
	return fmt.Errorf("%w: %s is missing a numeric field", application.ErrDownstreamUnavailable, where)
}

// isNumericalOverflow: the contract answers that case with a single issue, hence errors[0].
func isNumericalOverflow(raw []byte) bool {
	var doc struct {
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc.Errors) == 0 {
		return false
	}
	return doc.Errors[0].Code == string(matrix.CodeNumericalOverflow)
}

// Compute posts the labeled matrices. Retries cover connection failures, timeouts and 5xx
// only; a 4xx is a verdict, and ctx bounds the whole call, not one attempt.
func (c *Client) Compute(ctx context.Context, req application.StatisticsRequest) (application.StatisticsReport, error) {
	body, err := json.Marshal(struct {
		Matrices []application.LabeledMatrix `json:"matrices"`
	}{Matrices: req.Matrices})
	if err != nil {
		return application.StatisticsReport{}, fmt.Errorf("statsclient: encoding request: %w", err)
	}

	attempts := c.cfg.Retries + 1
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		report, retryable, err := c.attempt(ctx, req, body)
		if err == nil {
			return report, nil
		}
		lastErr = err
		if !retryable || attempt == attempts || !c.budgetAllowsAnotherAttempt(ctx) {
			break
		}
		c.cfg.Logger.WarnContext(ctx, "retrying statistics call",
			slog.Int("attempt", attempt), slog.String("error", err.Error()))
	}
	return application.StatisticsReport{}, lastErr
}

// budgetAllowsAnotherAttempt skips a retry the parent deadline cannot fit whole.
func (c *Client) budgetAllowsAnotherAttempt(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return true
	}
	return time.Until(deadline) >= c.cfg.Timeout
}

func (c *Client) attempt(ctx context.Context, req application.StatisticsRequest, body []byte) (application.StatisticsReport, bool, error) {
	// Derived from ctx, so the earlier deadline wins and an attempt never outlives the budget.
	attemptCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, c.cfg.BaseURL+"/api/v1/statistics", bytes.NewReader(body))
	if err != nil {
		return application.StatisticsReport{}, false, fmt.Errorf("%w: building request: %w", application.ErrDownstreamUnavailable, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", c.cfg.UserAgent)
	if req.Authorization != "" {
		httpReq.Header.Set("Authorization", req.Authorization)
	}
	if req.RequestID != "" {
		httpReq.Header.Set("X-Request-ID", req.RequestID)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		if isTimeout(attemptCtx, err) {
			return application.StatisticsReport{}, true, fmt.Errorf("%w: %w", application.ErrDownstreamTimeout, err)
		}
		return application.StatisticsReport{}, true, fmt.Errorf("%w: %w", application.ErrDownstreamUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		if isTimeout(attemptCtx, err) {
			return application.StatisticsReport{}, true, fmt.Errorf("%w: reading response: %w", application.ErrDownstreamTimeout, err)
		}
		return application.StatisticsReport{}, true, fmt.Errorf("%w: reading response: %w", application.ErrDownstreamUnavailable, err)
	}

	switch {
	case resp.StatusCode == http.StatusOK:
		var wire wireReport
		if err := json.Unmarshal(raw, &wire); err != nil {
			return application.StatisticsReport{}, false, fmt.Errorf("%w: unparseable body: %w", application.ErrDownstreamUnavailable, err)
		}
		report, err := wire.report()
		if err != nil {
			return application.StatisticsReport{}, false, err
		}
		return report, false, nil
	case resp.StatusCode >= 500:
		return application.StatisticsReport{}, true, fmt.Errorf("%w: status %d", application.ErrDownstreamUnavailable, resp.StatusCode)
	case resp.StatusCode == http.StatusUnprocessableEntity && isNumericalOverflow(raw):
		// stats-api cannot sum the caller's factors: the verdict belongs to the caller, as 422.
		return application.StatisticsReport{}, false, fmt.Errorf("%w: stats-api rejected the factors", application.ErrDownstreamNumericalOverflow)
	default:
		// 4xx, a rejected forwarded token included: never impersonated back, never retried.
		return application.StatisticsReport{}, false, fmt.Errorf("%w: status %d", application.ErrDownstreamUnavailable, resp.StatusCode)
	}
}

// Ping probes GET {base}/health/live for the readiness endpoint; ctx owns the deadline.
func (c *Client) Ping(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+"/health/live", http.NoBody)
	if err != nil {
		return fmt.Errorf("statsclient: building probe: %w", err)
	}
	httpReq.Header.Set("Accept", "application/health+json, application/json")
	httpReq.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: %w", application.ErrDownstreamUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", application.ErrDownstreamUnavailable, resp.StatusCode)
	}
	return nil
}

func isTimeout(ctx context.Context, err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
