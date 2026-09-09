package http

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
)

// healthContentType is the media type of both probes (draft-inadarei style).
const healthContentType = "application/health+json"

const (
	statusPass = "pass"
	statusFail = "fail"
)

// probeTimeout bounds the readiness probe: a slow dependency must not queue up requests.
const probeTimeout = time.Second

// probeCacheTTL keeps a result warm: N concurrent checks cost one call every 2 seconds.
const probeCacheTTL = 2 * time.Second

// healthCheck is one dependency probe of the health document.
type healthCheck struct {
	Status        string   `json:"status"`
	ComponentType string   `json:"componentType,omitempty"`
	ObservedValue *float64 `json:"observedValue,omitempty"`
	ObservedUnit  string   `json:"observedUnit,omitempty"`
	Time          string   `json:"time,omitempty"`
	Output        string   `json:"output,omitempty"`
}

// healthResponse is the body of /health/live and /health/ready.
type healthResponse struct {
	Status        string                 `json:"status"`
	Service       string                 `json:"service"`
	Version       string                 `json:"version"`
	UptimeSeconds float64                `json:"uptimeSeconds"`
	Checks        map[string]healthCheck `json:"checks,omitempty"`
}

// Pinger probes the downstream dependency.
type Pinger interface {
	Ping(ctx context.Context) error
}

// readinessCache serializes and memoizes the downstream probe.
type readinessCache struct {
	pinger Pinger
	ttl    time.Duration
	now    func() time.Time

	mu        sync.Mutex
	expiresAt time.Time
	cached    healthCheck
}

func newReadinessCache(pinger Pinger, now func() time.Time) *readinessCache {
	if now == nil {
		now = time.Now
	}
	return &readinessCache{pinger: pinger, ttl: probeCacheTTL, now: now}
}

// probeFailedOutput: /health/ready is unauthenticated, so the cause goes to the log only.
const probeFailedOutput = "stats-api probe failed"

// check returns the memoized probe; the error is non-nil only for a probe that just ran.
func (rc *readinessCache) check(ctx context.Context) (healthCheck, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	now := rc.now()
	if now.Before(rc.expiresAt) {
		return rc.cached, nil
	}

	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	err := rc.pinger.Ping(probeCtx)
	// The injected clock, not time.Now: freezing it makes observedValue deterministic.
	elapsed := float64(rc.now().Sub(now).Nanoseconds()) / 1e6

	check := healthCheck{
		Status:        statusPass,
		ComponentType: "datastore",
		ObservedValue: &elapsed,
		ObservedUnit:  "ms",
		Time:          now.UTC().Format("2006-01-02T15:04:05.000Z"),
	}
	if err != nil {
		check.Status = statusFail
		check.ObservedValue = nil
		check.ObservedUnit = ""
		check.Output = probeFailedOutput
	}

	rc.cached = check
	rc.expiresAt = now.Add(rc.ttl)
	return check, err
}

func (s *server) livenessHandler(c fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(healthResponse{
		Status:        statusPass,
		Service:       serviceName,
		Version:       s.version,
		UptimeSeconds: s.uptimeSeconds(),
	}, healthContentType)
}

func (s *server) readinessHandler(c fiber.Ctx) error {
	check, probeErr := s.readiness.check(c.Context())
	if probeErr != nil {
		s.deps.Logger.WarnContext(c.Context(), "readiness probe failed",
			slog.String("requestId", c.RequestID()),
			slog.String("component", "stats-api"),
			slog.String("error", probeErr.Error()))
	}

	status := fiber.StatusOK
	overall := statusPass
	if check.Status != statusPass {
		status = fiber.StatusServiceUnavailable
		overall = statusFail
	}

	return c.Status(status).JSON(healthResponse{
		Status:        overall,
		Service:       serviceName,
		Version:       s.version,
		UptimeSeconds: s.uptimeSeconds(),
		Checks:        map[string]healthCheck{"stats-api": check},
	}, healthContentType)
}
