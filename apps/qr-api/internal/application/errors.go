package application

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rodrigomm/proyectot/qr-api/internal/domain/matrix"
)

// MaxReportedViolations caps `errors[]` at 200 for both services (problems.md §3): a 100x100
// matrix can fail in every cell, and ten thousand issues make a multi-megabyte response.
const MaxReportedViolations = 200

// ErrValidation carries every failed rule of one request. -> 422 validation-error.
type ErrValidation struct {
	// Violations are the reported failures, at most MaxReportedViolations, by pointer.
	Violations []matrix.Violation
	// Total is how many rules failed; above len(Violations) exactly when truncated.
	Total int
}

func (e *ErrValidation) Error() string {
	if len(e.Violations) == 0 {
		return "validation failed"
	}
	parts := make([]string, 0, len(e.Violations))
	for _, v := range e.Violations {
		parts = append(parts, string(v.Code)+" at "+v.Pointer)
	}
	return "validation failed: " + strings.Join(parts, ", ")
}

// detailPrefix supplies context a per-field message lacks; wording frozen in the fixtures.
var detailPrefix = map[matrix.Code]string{
	matrix.CodeRaggedRow: "matrix must be rectangular: ",
}

// Detail returns the problem `detail` in the three shapes contracts/problems.md §3
// fixes for both services: one rule, its own message (prefixed per detailPrefix);
// several, their count; truncated, the count plus how many `errors[]` carries.
func (e *ErrValidation) Detail() string {
	switch {
	case len(e.Violations) == 0:
		return "The request body failed validation."
	case e.Total > len(e.Violations):
		return fmt.Sprintf("The request body contains %d validation errors; the first %d are listed.",
			e.Total, len(e.Violations))
	case e.Total > 1:
		return fmt.Sprintf("The request body contains %d validation errors.", e.Total)
	default:
		first := e.Violations[0]
		return detailPrefix[first.Code] + first.Message
	}
}

// NewValidationError sorts, truncates to MaxReportedViolations and keeps the total.
func NewValidationError(violations []matrix.Violation) *ErrValidation {
	matrix.SortViolations(violations)
	total := len(violations)
	if total > MaxReportedViolations {
		violations = violations[:MaxReportedViolations]
	}
	return &ErrValidation{Violations: violations, Total: total}
}

var (
	// ErrDownstreamUnavailable: stats-api unreachable, 5xx, rejected token or bad body. -> 502.
	ErrDownstreamUnavailable = errors.New("statistics service unavailable")
	// ErrDownstreamTimeout means every attempt exhausted STATS_API_TIMEOUT. -> 504.
	ErrDownstreamTimeout = errors.New("statistics service timed out")
	// ErrDownstreamNumericalOverflow: stats-api's 422, caused by the caller's matrix. -> 422.
	ErrDownstreamNumericalOverflow = errors.New("statistics overflow the float64 range")
	// ErrUnauthorized covers every credential and token failure; one error, no oracle. -> 401.
	ErrUnauthorized = errors.New("authentication required")
)
