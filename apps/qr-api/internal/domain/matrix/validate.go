package matrix

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Limits bounds the accepted input size, from MAX_MATRIX_ROWS and MAX_MATRIX_COLS.
type Limits struct {
	MaxRows int
	MaxCols int
}

// Validate returns a [Violation] per failed rule, ordered by pointer; every rule always runs.
func Validate(rows [][]float64, pointer string, limits Limits) []Violation {
	if len(rows) == 0 {
		return []Violation{{
			Pointer: pointer,
			Code:    CodeEmptyMatrix,
			Message: "matrix must have at least one row",
		}}
	}

	var out []Violation
	if limits.MaxRows > 0 && len(rows) > limits.MaxRows {
		out = append(out, Violation{
			Pointer: pointer,
			Code:    CodeTooManyRows,
			Message: fmt.Sprintf("matrix has %d rows, the maximum is %d", len(rows), limits.MaxRows),
		})
	}

	cols := len(rows[0])
	if limits.MaxCols > 0 && cols > limits.MaxCols {
		out = append(out, Violation{
			Pointer: pointer,
			Code:    CodeTooManyCols,
			Message: fmt.Sprintf("matrix has %d columns, the maximum is %d", cols, limits.MaxCols),
		})
	}

	for i, row := range rows {
		rowPointer := pointer + "/" + strconv.Itoa(i)
		switch {
		case len(row) == 0:
			out = append(out, Violation{
				Pointer: rowPointer,
				Code:    CodeEmptyRow,
				Message: fmt.Sprintf("row %d must have at least one column", i),
			})
			continue
		// cols == 0 means row 0 is empty and already reported; ragged_row on top would mislead.
		case i > 0 && cols > 0 && len(row) != cols:
			out = append(out, Violation{
				Pointer: rowPointer,
				Code:    CodeRaggedRow,
				Message: fmt.Sprintf("row %d has %d %s, expected %d", i, len(row), plural(len(row), "column"), cols),
			})
		}
		for j, v := range row {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				out = append(out, Violation{
					Pointer: rowPointer + "/" + strconv.Itoa(j),
					Code:    CodeNonFiniteValue,
					Message: fmt.Sprintf("cell [%d][%d] is not a finite number", i, j),
				})
			}
		}
	}

	SortViolations(out)
	return out
}

// plural picks the noun form so a message reads "1 column"; stats-api does the same.
func plural(count int, singular string) string {
	if count == 1 {
		return singular
	}
	return singular + "s"
}

// SortViolations orders by pointer, numeric segments compared numerically, stably.
func SortViolations(v []Violation) {
	sort.SliceStable(v, func(a, b int) bool { return comparePointers(v[a].Pointer, v[b].Pointer) < 0 })
}

func comparePointers(a, b string) int {
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if as[i] == bs[i] {
			continue
		}
		an, aErr := strconv.Atoi(as[i])
		bn, bErr := strconv.Atoi(bs[i])
		if aErr == nil && bErr == nil {
			if an != bn {
				if an < bn {
					return -1
				}
				return 1
			}
			continue
		}
		return strings.Compare(as[i], bs[i])
	}
	return len(as) - len(bs)
}
