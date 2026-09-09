package matrix

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

const testPointer = "/matrix"

var testLimits = Limits{MaxRows: 100, MaxCols: 100}

func TestValidate_Accepts(t *testing.T) {
	t.Parallel()

	for _, rows := range [][][]float64{
		{{1}},
		{{1, 2}, {3, 4}, {5, 6}},
		{{1, 2, 3}},
		{{0, 0}, {0, 0}},
		{{-1e308, 1e-308}},
	} {
		require.Empty(t, Validate(rows, testPointer, testLimits))
	}
}

func TestValidate_OneViolationPerRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rows    [][]float64
		limits  Limits
		want    []Violation
		partial bool
	}{
		{
			name: "empty matrix",
			rows: [][]float64{},
			want: []Violation{{Pointer: "/matrix", Code: CodeEmptyMatrix, Message: "matrix must have at least one row"}},
		},
		{
			name: "nil matrix",
			rows: nil,
			want: []Violation{{Pointer: "/matrix", Code: CodeEmptyMatrix, Message: "matrix must have at least one row"}},
		},
		{
			name: "empty first row",
			rows: [][]float64{{}},
			want: []Violation{{Pointer: "/matrix/0", Code: CodeEmptyRow, Message: "row 0 must have at least one column"}},
		},
		{
			name: "empty later row",
			rows: [][]float64{{1, 2}, {}},
			want: []Violation{{Pointer: "/matrix/1", Code: CodeEmptyRow, Message: "row 1 must have at least one column"}},
		},
		{
			// cols is 0, so only empty_row is reported; ragged_row on top would be noise.
			name: "empty first row does not make the later rows ragged",
			rows: [][]float64{{}, {1, 2}},
			want: []Violation{{Pointer: "/matrix/0", Code: CodeEmptyRow, Message: "row 0 must have at least one column"}},
		},
		{
			name: "ragged row",
			rows: [][]float64{{1, 2}, {1, 2, 3}},
			want: []Violation{{Pointer: "/matrix/1", Code: CodeRaggedRow, Message: "row 1 has 3 columns, expected 2"}},
		},
		{
			name: "ragged shorter row",
			rows: [][]float64{{1, 2}, {3}},
			want: []Violation{{Pointer: "/matrix/1", Code: CodeRaggedRow, Message: "row 1 has 1 column, expected 2"}},
		},
		{
			name: "positive infinity",
			rows: [][]float64{{1, math.Inf(1)}},
			want: []Violation{{Pointer: "/matrix/0/1", Code: CodeNonFiniteValue, Message: "cell [0][1] is not a finite number"}},
		},
		{
			name: "NaN",
			rows: [][]float64{{math.NaN()}},
			want: []Violation{{Pointer: "/matrix/0/0", Code: CodeNonFiniteValue, Message: "cell [0][0] is not a finite number"}},
		},
		{
			name:   "too many rows",
			rows:   [][]float64{{1}, {2}, {3}},
			limits: Limits{MaxRows: 2, MaxCols: 100},
			want:   []Violation{{Pointer: "/matrix", Code: CodeTooManyRows, Message: "matrix has 3 rows, the maximum is 2"}},
		},
		{
			name:   "too many cols",
			rows:   [][]float64{{1, 2, 3}},
			limits: Limits{MaxRows: 100, MaxCols: 2},
			want:   []Violation{{Pointer: "/matrix", Code: CodeTooManyCols, Message: "matrix has 3 columns, the maximum is 2"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			limits := tc.limits
			if limits == (Limits{}) {
				limits = testLimits
			}
			require.Equal(t, tc.want, Validate(tc.rows, testPointer, limits))
		})
	}
}

func TestValidate_ReportsEveryFailedRuleOrderedByPointer(t *testing.T) {
	t.Parallel()

	rows := [][]float64{
		{1, math.Inf(-1)},
		{1, 2, 3},
		{math.NaN(), 2},
	}
	got := Validate(rows, testPointer, testLimits)
	require.Equal(t, []Violation{
		{Pointer: "/matrix/0/1", Code: CodeNonFiniteValue, Message: "cell [0][1] is not a finite number"},
		{Pointer: "/matrix/1", Code: CodeRaggedRow, Message: "row 1 has 3 columns, expected 2"},
		{Pointer: "/matrix/2/0", Code: CodeNonFiniteValue, Message: "cell [2][0] is not a finite number"},
	}, got)
}

func TestSortViolations_NumericSegmentsSortNumerically(t *testing.T) {
	t.Parallel()

	got := []Violation{
		{Pointer: "/mode"},
		{Pointer: "/matrix/10"},
		{Pointer: "/matrix/2/1"},
		{Pointer: "/matrix"},
		{Pointer: "/matrix/2"},
		{Pointer: "/clientSecret"},
	}
	SortViolations(got)

	pointers := make([]string, 0, len(got))
	for _, v := range got {
		pointers = append(pointers, v.Pointer)
	}
	require.Equal(t, []string{
		"/clientSecret", "/matrix", "/matrix/2", "/matrix/2/1", "/matrix/10", "/mode",
	}, pointers)
}

func TestSortViolations_IsStable(t *testing.T) {
	t.Parallel()

	got := []Violation{
		{Pointer: "/matrix", Code: CodeTooManyRows},
		{Pointer: "/matrix", Code: CodeTooManyCols},
	}
	SortViolations(got)
	require.Equal(t, CodeTooManyRows, got[0].Code)
	require.Equal(t, CodeTooManyCols, got[1].Code)
}
