package application_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/application"
	"github.com/rodrigomm/proyectot/qr-api/internal/domain/matrix"
)

// fakeStatistics records every call, so "validation never calls downstream" is an assertion.
type fakeStatistics struct {
	calls       []application.StatisticsRequest
	report      application.StatisticsReport
	err         error
	deadline    time.Time
	hasDeadline bool
}

func (f *fakeStatistics) Compute(ctx context.Context, req application.StatisticsRequest) (application.StatisticsReport, error) {
	f.calls = append(f.calls, req)
	f.deadline, f.hasDeadline = ctx.Deadline()
	if f.err != nil {
		return application.StatisticsReport{}, f.err
	}
	return f.report, nil
}

func sampleReport() application.StatisticsReport {
	return application.StatisticsReport{
		Summary: application.StatisticsSummary{Max: 1, Min: 0, Sum: 6, Average: 0.5, Count: 12, AnyDiagonal: true},
		Matrices: []application.MatrixStatistics{
			{Index: 0, Label: "Q", Rows: 3, Cols: 3},
			{Index: 1, Label: "R", Rows: 3, Cols: 2},
		},
		Meta: application.StatisticsMeta{SummationAlgorithm: "neumaier", ElapsedMs: 0.2},
	}
}

// testBudget mirrors config.DefaultRequestBudget; application must not import platform.
const testBudget = 12 * time.Second

func newUseCase(stats application.StatisticsClient) *application.DecomposeAndAnalyze {
	return application.NewDecomposeAndAnalyze(stats, matrix.Limits{MaxRows: 100, MaxCols: 100}, testBudget, nil)
}

func TestDecomposeAndAnalyze_HappyPath(t *testing.T) {
	t.Parallel()

	stats := &fakeStatistics{report: sampleReport()}
	result, err := newUseCase(stats).Execute(context.Background(), application.DecomposeCommand{
		Rows:          [][]float64{{1, 2}, {3, 4}, {5, 6}},
		Authorization: "Bearer token-123",
		RequestID:     "req-1",
	})
	require.NoError(t, err)

	require.Equal(t, 3, result.InputRows)
	require.Equal(t, 2, result.InputCols)
	require.Len(t, result.Q, 3)
	require.Len(t, result.Q[0], 3)
	require.Len(t, result.R, 3)
	require.Len(t, result.R[0], 2)
	require.Equal(t, sampleReport(), result.Statistics)
	require.GreaterOrEqual(t, result.DecompositionMs, 0.0)
	require.GreaterOrEqual(t, result.StatisticsMs, 0.0)

	require.Len(t, stats.calls, 1)
	call := stats.calls[0]
	require.Equal(t, "Bearer token-123", call.Authorization, "the caller's token is forwarded verbatim")
	require.Equal(t, "req-1", call.RequestID)
	require.Len(t, call.Matrices, 2)
	require.Equal(t, "Q", call.Matrices[0].Label)
	require.Equal(t, "R", call.Matrices[1].Label)
	require.Equal(t, result.Q, call.Matrices[0].Values)
	require.Equal(t, result.R, call.Matrices[1].Values)
}

func TestDecomposeAndAnalyze_ReducedMode(t *testing.T) {
	t.Parallel()

	stats := &fakeStatistics{report: sampleReport()}
	result, err := newUseCase(stats).Execute(context.Background(), application.DecomposeCommand{
		Rows: [][]float64{{1, 2}, {3, 4}, {5, 6}},
		Mode: "reduced",
	})
	require.NoError(t, err)
	require.Len(t, result.Q, 3)
	require.Len(t, result.Q[0], 2)
	require.Len(t, result.R, 2)
}

func TestDecomposeAndAnalyze_ValidationNeverCallsDownstream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  application.DecomposeCommand
		want []matrix.Violation
	}{
		{
			name: "empty matrix",
			cmd:  application.DecomposeCommand{Rows: [][]float64{}},
			want: []matrix.Violation{{Pointer: "/matrix", Code: matrix.CodeEmptyMatrix, Message: "matrix must have at least one row"}},
		},
		{
			name: "ragged row",
			cmd:  application.DecomposeCommand{Rows: [][]float64{{1, 2}, {3}}},
			want: []matrix.Violation{{Pointer: "/matrix/1", Code: matrix.CodeRaggedRow, Message: "row 1 has 1 column, expected 2"}},
		},
		{
			name: "invalid mode",
			cmd:  application.DecomposeCommand{Rows: [][]float64{{1}}, Mode: "thin"},
			want: []matrix.Violation{{Pointer: "/mode", Code: matrix.CodeInvalidMode, Message: `mode must be "full" or "reduced"`}},
		},
		{
			name: "decode issues are merged and ordered",
			cmd: application.DecomposeCommand{
				Rows: [][]float64{{0, 2}},
				Mode: "thin",
				DecodeIssues: []matrix.Violation{
					{Pointer: "/matrix/0/0", Code: matrix.CodeInvalidType, Message: "cell [0][0] must be a number"},
				},
			},
			want: []matrix.Violation{
				{Pointer: "/matrix/0/0", Code: matrix.CodeInvalidType, Message: "cell [0][0] must be a number"},
				{Pointer: "/mode", Code: matrix.CodeInvalidMode, Message: `mode must be "full" or "reduced"`},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stats := &fakeStatistics{report: sampleReport()}
			_, err := newUseCase(stats).Execute(context.Background(), tc.cmd)

			var validation *application.ErrValidation
			require.ErrorAs(t, err, &validation)
			require.Equal(t, tc.want, validation.Violations)
			require.Empty(t, stats.calls, "a rejected request must never reach stats-api")
		})
	}
}

func TestDecomposeAndAnalyze_NumericalOverflowIsAValidationProblem(t *testing.T) {
	t.Parallel()

	stats := &fakeStatistics{report: sampleReport()}
	_, err := newUseCase(stats).Execute(context.Background(), application.DecomposeCommand{
		Rows: [][]float64{{1.7976931348623157e308, 1.7976931348623157e308}, {1.7976931348623157e308, 1.7976931348623157e308}},
	})

	var validation *application.ErrValidation
	require.ErrorAs(t, err, &validation)
	require.Len(t, validation.Violations, 1)
	require.Equal(t, matrix.CodeNumericalOverflow, validation.Violations[0].Code)
	require.Equal(t, "/matrix", validation.Violations[0].Pointer)
	require.Empty(t, stats.calls)
}

func TestDecomposeAndAnalyze_DownstreamErrorsAreForwarded(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
	}{
		{"timeout", application.ErrDownstreamTimeout},
		{"unavailable", application.ErrDownstreamUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stats := &fakeStatistics{err: tc.err}
			_, err := newUseCase(stats).Execute(context.Background(), application.DecomposeCommand{
				Rows: [][]float64{{1, 2}, {3, 4}},
			})
			require.ErrorIs(t, err, tc.err)
			require.Len(t, stats.calls, 1)
		})
	}
}

func TestDecomposeAndAnalyze_DownstreamOverflowBecomesAValidationError(t *testing.T) {
	t.Parallel()

	// The dependency is healthy; the caller's values are the cause, so 422 and not 502.
	stats := &fakeStatistics{err: application.ErrDownstreamNumericalOverflow}
	_, err := newUseCase(stats).Execute(context.Background(), application.DecomposeCommand{
		Rows: [][]float64{{1, 2}, {3, 4}},
	})

	var validation *application.ErrValidation
	require.ErrorAs(t, err, &validation)
	require.NotErrorIs(t, err, application.ErrDownstreamUnavailable)
	require.Equal(t, []matrix.Violation{{
		Pointer: "/matrix",
		Code:    matrix.CodeNumericalOverflow,
		Message: "the values of the decomposition overflow the float64 range; their sum cannot be represented",
	}}, validation.Violations)
	require.Len(t, stats.calls, 1)
}

func TestDecomposeAndAnalyze_MeasuresWithTheInjectedClock(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	ticks := 0
	clock := func() time.Time {
		ticks++
		return now.Add(time.Duration(ticks) * time.Millisecond)
	}

	uc := application.NewDecomposeAndAnalyze(&fakeStatistics{report: sampleReport()},
		matrix.Limits{MaxRows: 100, MaxCols: 100}, testBudget, clock)
	result, err := uc.Execute(context.Background(), application.DecomposeCommand{Rows: [][]float64{{1}}})
	require.NoError(t, err)
	require.InDelta(t, 1.0, result.DecompositionMs, 1e-9)
	require.InDelta(t, 1.0, result.StatisticsMs, 1e-9)
}

func TestValidationError_Message(t *testing.T) {
	t.Parallel()

	empty := &application.ErrValidation{}
	require.Equal(t, "validation failed", empty.Error())
	require.Equal(t, "The request body failed validation.", empty.Detail())

	err := application.NewValidationError([]matrix.Violation{
		{Pointer: "/mode", Code: matrix.CodeInvalidMode, Message: "bad mode"},
		{Pointer: "/matrix", Code: matrix.CodeEmptyMatrix, Message: "empty"},
	})
	require.Equal(t, "The request body contains 2 validation errors.", err.Detail(),
		"no single message describes two failures")
	require.Equal(t, "validation failed: empty_matrix at /matrix, invalid_mode at /mode", err.Error())
	require.True(t, errors.Is(err, err))
}

// Pins the three sentences of problems.md §3; stats-api renders the identical ones.
func TestValidationError_Detail(t *testing.T) {
	t.Parallel()

	single := application.NewValidationError([]matrix.Violation{
		{Pointer: "/matrix", Code: matrix.CodeEmptyMatrix, Message: "matrix must have at least one row"},
	})
	require.Equal(t, "matrix must have at least one row", single.Detail(),
		"one failure speaks for itself")

	ragged := application.NewValidationError([]matrix.Violation{
		{Pointer: "/matrix/1", Code: matrix.CodeRaggedRow, Message: "row 1 has 3 columns, expected 2"},
	})
	require.Equal(t, "matrix must be rectangular: row 1 has 3 columns, expected 2", ragged.Detail(),
		"the only per-code prefix, kept for the golden fixture")

	many := make([]matrix.Violation, 0, 250)
	for i := range 250 {
		many = append(many, matrix.Violation{
			Pointer: "/matrix/0/" + strconv.Itoa(i),
			Code:    matrix.CodeInvalidType,
			Message: "cell must be a number",
		})
	}
	truncated := application.NewValidationError(many)
	require.Len(t, truncated.Violations, application.MaxReportedViolations)
	require.Equal(t, "The request body contains 250 validation errors; the first 200 are listed.",
		truncated.Detail())
}

func TestDecomposeAndAnalyze_AppliesTheRequestBudget(t *testing.T) {
	t.Parallel()

	stats := &fakeStatistics{report: sampleReport()}
	_, err := newUseCase(stats).Execute(context.Background(), application.DecomposeCommand{Rows: [][]float64{{1, 2}, {3, 4}}})
	require.NoError(t, err)

	require.True(t, stats.hasDeadline, "the downstream call inherits the request budget")
	remaining := time.Until(stats.deadline)
	require.LessOrEqual(t, remaining, testBudget)
	require.Greater(t, remaining, testBudget-time.Second)
}

func TestDecomposeAndAnalyze_KeepsATighterCallerDeadline(t *testing.T) {
	t.Parallel()

	// The budget is a ceiling, never an extension: a shorter caller deadline stands.
	stats := &fakeStatistics{report: sampleReport()}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := newUseCase(stats).Execute(ctx, application.DecomposeCommand{Rows: [][]float64{{1, 2}, {3, 4}}})
	require.NoError(t, err)
	require.True(t, stats.hasDeadline)
	require.LessOrEqual(t, time.Until(stats.deadline), 50*time.Millisecond)
}

func TestDecomposeAndAnalyze_ZeroBudgetLeavesTheDeadlineToTheCaller(t *testing.T) {
	t.Parallel()

	stats := &fakeStatistics{report: sampleReport()}
	uc := application.NewDecomposeAndAnalyze(stats, matrix.Limits{MaxRows: 100, MaxCols: 100}, 0, nil)
	_, err := uc.Execute(context.Background(), application.DecomposeCommand{Rows: [][]float64{{1}}})
	require.NoError(t, err)
	require.False(t, stats.hasDeadline)
}
