package matrix

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatrix_Accessors(t *testing.T) {
	t.Parallel()

	m := FromRows([][]float64{{1, 2, 3}, {4, 5, 6}})
	require.Equal(t, 2, m.Rows())
	require.Equal(t, 3, m.Cols())
	require.Equal(t, 5.0, m.At(1, 1))

	m.Set(1, 1, 50)
	require.Equal(t, 50.0, m.At(1, 1))
	require.Equal(t, [][]float64{{1, 2, 3}, {4, 50, 6}}, m.Rows2D())
}

func TestMatrix_CloneIsDeep(t *testing.T) {
	t.Parallel()

	m := FromRows([][]float64{{1, 2}, {3, 4}})
	c := m.Clone()
	c.Set(0, 0, 99)
	require.Equal(t, 1.0, m.At(0, 0))
	require.Equal(t, 99.0, c.At(0, 0))
}

func TestMatrix_Rows2DIsACopy(t *testing.T) {
	t.Parallel()

	m := FromRows([][]float64{{1, 2}})
	rows := m.Rows2D()
	rows[0][0] = 99
	require.Equal(t, 1.0, m.At(0, 0))
}

func TestIdentityAndSubmatrix(t *testing.T) {
	t.Parallel()

	require.Equal(t, [][]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}, Identity(3).Rows2D())
	m := FromRows([][]float64{{1, 2, 3}, {4, 5, 6}, {7, 8, 9}})
	require.Equal(t, [][]float64{{1, 2}, {4, 5}, {7, 8}}, m.Submatrix(3, 2).Rows2D())
	require.Equal(t, [][]float64{{1, 2, 3}}, m.Submatrix(1, 3).Rows2D())
}

func TestMatrix_IsFiniteAndNormalizeZeros(t *testing.T) {
	t.Parallel()

	m := FromRows([][]float64{{1, math.Inf(1)}})
	require.False(t, m.IsFinite())
	m.Set(0, 1, math.NaN())
	require.False(t, m.IsFinite())
	m.Set(0, 1, 2)
	require.True(t, m.IsFinite())

	z := FromRows([][]float64{{math.Copysign(0, -1), 0, -1}})
	require.True(t, math.Signbit(z.At(0, 0)))
	z.NormalizeZeros()
	require.False(t, math.Signbit(z.At(0, 0)), "negative zero must become positive zero")
	require.Equal(t, -1.0, z.At(0, 2), "non-zero entries keep their sign")
}

func TestMatrix_AbsRangeAndScaleByPow2(t *testing.T) {
	t.Parallel()

	m := FromRows([][]float64{{1, -7}, {3, 2}})
	minNonZero, maxAbs := m.AbsRange()
	require.Equal(t, 1.0, minNonZero)
	require.Equal(t, 7.0, maxAbs)

	m.ScaleByPow2(0)
	require.Equal(t, 1.0, m.At(0, 0))
	m.ScaleByPow2(3)
	require.Equal(t, 8.0, m.At(0, 0))
	require.Equal(t, -56.0, m.At(0, 1))
	m.ScaleByPow2(-3)
	require.Equal(t, [][]float64{{1, -7}, {3, 2}}, m.Rows2D(), "power-of-two scaling round-trips exactly")
}

func TestMatrix_AbsRangeIgnoresZerosAndSigns(t *testing.T) {
	t.Parallel()

	minNonZero, maxAbs := FromRows([][]float64{{0, -1e-310}, {1e308, 0}}).AbsRange()
	require.Equal(t, 1e-310, minNonZero, "zeros never become the lower bound, signs are ignored")
	require.Equal(t, 1e308, maxAbs)

	minNonZero, maxAbs = FromRows([][]float64{{0, 0}}).AbsRange()
	require.Zero(t, minNonZero, "an all-zero matrix has no non-zero magnitude")
	require.Zero(t, maxAbs)
}

func TestMatrix_PanicsOnInvalidInput(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() { New(0, 3) })
	require.Panics(t, func() { New(3, 0) })
	require.Panics(t, func() { FromRows(nil) })
	require.Panics(t, func() { FromRows([][]float64{{}}) })
	require.Panics(t, func() { FromRows([][]float64{{1, 2}, {3}}) })
}
