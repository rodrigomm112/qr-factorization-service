package qr

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/domain/matrix"
)

// FuzzDecompose: every finite matrix honours the invariants or fails ErrNumericalOverflow.
func FuzzDecompose(f *testing.F) {
	f.Add(2, 2, 1.0, 2.0, 3.0, 4.0)
	f.Add(3, 2, 1.0, 2.0, 3.0, 4.0)
	f.Add(1, 4, 0.0, 0.0, 0.0, 0.0)
	f.Add(4, 1, 1e150, -1e-150, 0.0, 1.0)
	f.Add(2, 3, -1.0, math.SmallestNonzeroFloat64, math.MaxFloat64/4, 0.5)

	f.Fuzz(func(t *testing.T, rows, cols int, a, b, c, d float64) {
		rows = 1 + abs(rows)%6
		cols = 1 + abs(cols)%6
		seeds := []float64{a, b, c, d}
		for _, v := range seeds {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Skip("the transport layer rejects non-finite cells before the domain sees them")
			}
		}

		values := make([][]float64, rows)
		for i := range values {
			values[i] = make([]float64, cols)
			for j := range values[i] {
				values[i][j] = seeds[(i*cols+j)%len(seeds)]
			}
		}

		m := matrix.FromRows(values)
		for _, mode := range []Mode{ModeFull, ModeReduced} {
			decomposition, err := Decompose(m, mode)
			if err != nil {
				require.ErrorIs(t, err, ErrNumericalOverflow)
				continue
			}
			assertQRInvariants(t, m, decomposition)
		}
	})
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
