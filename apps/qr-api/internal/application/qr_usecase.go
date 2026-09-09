package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/rodrigomm/proyectot/qr-api/internal/domain/matrix"
	"github.com/rodrigomm/proyectot/qr-api/internal/domain/qr"
)

const (
	pointerMatrix = "/matrix"
	pointerMode   = "/mode"
)

// DecomposeCommand is the input of [DecomposeAndAnalyze].
type DecomposeCommand struct {
	// Rows is the decoded matrix, unused when DecodeIssues is non-empty.
	Rows [][]float64
	// Mode is the raw wire value; "" means the default (full).
	Mode string
	// DecodeIssues are the decoder's violations, merged with the domain rules for one 422.
	DecodeIssues []matrix.Violation
	// Authorization and RequestID are forwarded verbatim to stats-api.
	Authorization string
	RequestID     string
}

// DecomposeResult is the output of [DecomposeAndAnalyze].
type DecomposeResult struct {
	InputRows       int
	InputCols       int
	Q               [][]float64
	R               [][]float64
	Mode            qr.Mode
	Statistics      StatisticsReport
	DecompositionMs float64
	StatisticsMs    float64
}

// DecomposeAndAnalyze validates, factors and enriches; validation is the single gate.
type DecomposeAndAnalyze struct {
	stats  StatisticsClient
	limits matrix.Limits
	budget time.Duration
	now    func() time.Time
}

// NewDecomposeAndAnalyze wires the use case; budget <= 0 defers to the caller's context.
func NewDecomposeAndAnalyze(stats StatisticsClient, limits matrix.Limits, budget time.Duration, now func() time.Time) *DecomposeAndAnalyze {
	if now == nil {
		now = time.Now
	}
	return &DecomposeAndAnalyze{stats: stats, limits: limits, budget: budget, now: now}
}

// Execute returns *ErrValidation, ErrDownstreamTimeout or ErrDownstreamUnavailable; else 500.
func (uc *DecomposeAndAnalyze) Execute(ctx context.Context, cmd DecomposeCommand) (*DecomposeResult, error) { //nolint:gocritic // one command per request; the copy is cheap next to the decomposition
	// One deadline for the whole request: decomposition and the stats call (retries included)
	// share the budget. Otherwise the worst case, decomposition + STATS_API_TIMEOUT x (1 +
	// retries), outlives the write timeout and the caller gets nothing.
	if uc.budget > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, uc.budget)
		defer cancel()
	}

	violations := slices.Clone(cmd.DecodeIssues)

	mode, ok := qr.ParseMode(cmd.Mode)
	if !ok {
		violations = append(violations, matrix.Violation{
			Pointer: pointerMode,
			Code:    matrix.CodeInvalidMode,
			Message: fmt.Sprintf("mode must be %q or %q", qr.ModeFull, qr.ModeReduced),
		})
	}
	if len(cmd.DecodeIssues) == 0 {
		violations = append(violations, matrix.Validate(cmd.Rows, pointerMatrix, uc.limits)...)
	}
	if len(violations) > 0 {
		return nil, NewValidationError(violations)
	}

	a := matrix.FromRows(cmd.Rows)

	start := uc.now()
	decomposition, err := qr.Decompose(a, mode)
	decompositionMs := millisSince(uc.now(), start)
	if err != nil {
		if errors.Is(err, qr.ErrNumericalOverflow) {
			return nil, NewValidationError([]matrix.Violation{{
				Pointer: pointerMatrix,
				Code:    matrix.CodeNumericalOverflow,
				Message: "the decomposition produced a non-finite entry; the matrix is too ill-scaled",
			}})
		}
		return nil, err
	}

	q, r := decomposition.Q.Rows2D(), decomposition.R.Rows2D()

	start = uc.now()
	report, err := uc.stats.Compute(ctx, StatisticsRequest{
		Matrices: []LabeledMatrix{
			{Label: "Q", Values: q},
			{Label: "R", Values: r},
		},
		Authorization: cmd.Authorization,
		RequestID:     cmd.RequestID,
	})
	statisticsMs := millisSince(uc.now(), start)
	if err != nil {
		if errors.Is(err, ErrDownstreamNumericalOverflow) {
			return nil, NewValidationError([]matrix.Violation{{
				Pointer: pointerMatrix,
				Code:    matrix.CodeNumericalOverflow,
				Message: "the values of the decomposition overflow the float64 range; their sum cannot be represented",
			}})
		}
		return nil, err
	}

	return &DecomposeResult{
		InputRows:       a.Rows(),
		InputCols:       a.Cols(),
		Q:               q,
		R:               r,
		Mode:            decomposition.Mode,
		Statistics:      report,
		DecompositionMs: decompositionMs,
		StatisticsMs:    statisticsMs,
	}, nil
}

func millisSince(end, start time.Time) float64 {
	return float64(end.Sub(start).Nanoseconds()) / 1e6
}
