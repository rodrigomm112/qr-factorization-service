package qr

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/domain/matrix"
)

// invariantTolerance is the floor of the residual checks: Householder is backward stable, so
// the error grows like O(u*||A||_F) with u = 2^-52 and the tolerance scales with ||A||_F.
const invariantTolerance = 1e-12

func assertQRInvariants(t *testing.T, a matrix.Matrix, d Decomposition) {
	t.Helper()

	m, n := a.Rows(), a.Cols()
	k := min(m, n)

	switch d.Mode {
	case ModeReduced:
		require.Equal(t, m, d.Q.Rows(), "reduced Q must be m x k")
		require.Equal(t, k, d.Q.Cols(), "reduced Q must be m x k")
		require.Equal(t, k, d.R.Rows(), "reduced R must be k x n")
		require.Equal(t, n, d.R.Cols(), "reduced R must be k x n")
	default:
		require.Equal(t, m, d.Q.Rows(), "full Q must be m x m")
		require.Equal(t, m, d.Q.Cols(), "full Q must be m x m")
		require.Equal(t, m, d.R.Rows(), "full R must be m x n")
		require.Equal(t, n, d.R.Cols(), "full R must be m x n")
	}

	scale := math.Max(1, frobenius(a))

	// 1. A = Q*R.
	residual := 0.0
	for i := range m {
		for j := range n {
			sum := 0.0
			for p := range d.Q.Cols() {
				sum += d.Q.At(i, p) * d.R.At(p, j)
			}
			diff := sum - a.At(i, j)
			residual += diff * diff
		}
	}
	require.LessOrEqual(t, math.Sqrt(residual), invariantTolerance*scale, "||QR - A||_F")

	// 2. Q has orthonormal columns.
	orth := 0.0
	for i := range d.Q.Cols() {
		for j := range d.Q.Cols() {
			sum := 0.0
			for p := range d.Q.Rows() {
				sum += d.Q.At(p, i) * d.Q.At(p, j)
			}
			expected := 0.0
			if i == j {
				expected = 1
			}
			orth += (sum - expected) * (sum - expected)
		}
	}
	require.LessOrEqual(t, math.Sqrt(orth), invariantTolerance, "||QtQ - I||_F")

	// 3. R is upper triangular, diagonal non-negative, no negative zero, all finite.
	for i := range d.R.Rows() {
		for j := range d.R.Cols() {
			v := d.R.At(i, j)
			require.False(t, math.IsNaN(v) || math.IsInf(v, 0), "R[%d][%d] must be finite", i, j)
			require.False(t, isNegativeZero(v), "R[%d][%d] must not be negative zero", i, j)
			if i > j {
				require.Equal(t, 0.0, v, "R[%d][%d] must be an exact zero", i, j)
			}
			if i == j {
				require.GreaterOrEqual(t, v, 0.0, "R[%d][%d] must be non-negative", i, j)
			}
		}
	}
	for i := range d.Q.Rows() {
		for j := range d.Q.Cols() {
			v := d.Q.At(i, j)
			require.False(t, math.IsNaN(v) || math.IsInf(v, 0), "Q[%d][%d] must be finite", i, j)
			require.False(t, isNegativeZero(v), "Q[%d][%d] must not be negative zero", i, j)
		}
	}
}

func isNegativeZero(v float64) bool { return v == 0 && math.Signbit(v) }

func frobenius(m matrix.Matrix) float64 {
	sum := 0.0
	for i := range m.Rows() {
		for j := range m.Cols() {
			v := m.At(i, j)
			sum += v * v
		}
	}
	return math.Sqrt(sum)
}

// decompose is the shorthand used by every table-driven case.
func decompose(t *testing.T, rows [][]float64, mode Mode) (matrix.Matrix, Decomposition) {
	t.Helper()
	a := matrix.FromRows(rows)
	d, err := Decompose(a, mode)
	require.NoError(t, err)
	assertQRInvariants(t, a, d)
	return a, d
}
