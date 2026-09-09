package qr

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/domain/matrix"
)

func TestDecompose_NamedCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		rows [][]float64
	}{
		{"identity 3x3", [][]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}},
		{"diagonal positive", [][]float64{{2, 0, 0}, {0, 3, 0}, {0, 0, 4}}},
		{"diagonal negative", [][]float64{{-2, 0, 0}, {0, -3, 0}, {0, 0, -4}}},
		{"1x1 positive", [][]float64{{7}}},
		{"1x1 negative", [][]float64{{-7}}},
		{"1x1 zero", [][]float64{{0}}},
		{"tall 3x2", [][]float64{{1, 2}, {3, 4}, {5, 6}}},
		{"wide 2x3", [][]float64{{1, 2, 3}, {4, 5, 6}}},
		{"row vector 1x5", [][]float64{{1, 2, 3, 4, 5}}},
		{"column vector 10x1", [][]float64{{1}, {2}, {3}, {4}, {5}, {6}, {7}, {8}, {9}, {10}}},
		{"permutation", [][]float64{{0, 1, 0}, {0, 0, 1}, {1, 0, 0}}},
		{"rank deficient", [][]float64{{1, 2}, {2, 4}, {3, 6}}},
		{"zero matrix", [][]float64{{0, 0}, {0, 0}, {0, 0}}},
		{"first column zero", [][]float64{{0, 1}, {0, 2}, {0, 3}}},
		{"all negative", [][]float64{{-1, -2}, {-3, -4}, {-5, -6}}},
		{"huge magnitude 1e150", [][]float64{{1e150, 2e150}, {3e150, 4e150}}},
		{"tiny magnitude 1e-150", [][]float64{{1e-150, 2e-150}, {3e-150, 4e-150}}},
		{"hilbert 5x5", hilbert(5)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			decompose(t, tc.rows, ModeFull)
		})
		t.Run(tc.name+"/reduced", func(t *testing.T) {
			t.Parallel()
			decompose(t, tc.rows, ModeReduced)
		})
	}
}

func TestDecompose_IdentityIsExact(t *testing.T) {
	t.Parallel()

	_, d := decompose(t, [][]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}, ModeFull)
	require.Equal(t, [][]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}, d.Q.Rows2D())
	require.Equal(t, [][]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}, d.R.Rows2D())
}

func TestDecompose_1x1(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		value float64
		wantQ float64
		wantR float64
	}{
		{"positive", 7, 1, 7},
		{"negative", -7, -1, 7},
		{"zero", 0, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, d := decompose(t, [][]float64{{tc.value}}, ModeFull)
			require.Equal(t, tc.wantQ, d.Q.At(0, 0))
			require.Equal(t, tc.wantR, d.R.At(0, 0))
		})
	}
}

func TestDecompose_ZeroMatrixKeepsIdentityQ(t *testing.T) {
	t.Parallel()

	_, d := decompose(t, [][]float64{{0, 0}, {0, 0}, {0, 0}}, ModeFull)
	require.Equal(t, matrix.Identity(3).Rows2D(), d.Q.Rows2D())
	for i := range 3 {
		for j := range 2 {
			require.Equal(t, 0.0, d.R.At(i, j))
		}
	}
}

func TestDecompose_ReducedShapes(t *testing.T) {
	t.Parallel()

	t.Run("tall", func(t *testing.T) {
		t.Parallel()
		_, d := decompose(t, [][]float64{{1, 2}, {3, 4}, {5, 6}}, ModeReduced)
		require.Equal(t, 3, d.Q.Rows())
		require.Equal(t, 2, d.Q.Cols())
		require.Equal(t, 2, d.R.Rows())
		require.Equal(t, 2, d.R.Cols())
	})

	t.Run("wide", func(t *testing.T) {
		t.Parallel()
		_, d := decompose(t, [][]float64{{1, 2, 3}, {4, 5, 6}}, ModeReduced)
		require.Equal(t, 2, d.Q.Rows())
		require.Equal(t, 2, d.Q.Cols())
		require.Equal(t, 2, d.R.Rows())
		require.Equal(t, 3, d.R.Cols())
	})
}

// k = min(m, n) == m, so the thin factors are the full ones: same numbers, no submatrix
// copies. Not parallel: testing.AllocsPerRun pins GOMAXPROCS to 1.
func TestDecompose_ReducedIsFullWhenRowsDoNotExceedColumns(t *testing.T) {
	cases := []struct {
		name string
		rows [][]float64
	}{
		{"square 3x3", [][]float64{{1, 2, 3}, {4, 5, 6}, {7, 8, 10}}},
		{"wide 2x3", [][]float64{{1, 2, 3}, {4, 5, 6}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := matrix.FromRows(tc.rows)

			full, err := Decompose(a, ModeFull)
			require.NoError(t, err)
			reduced, err := Decompose(a, ModeReduced)
			require.NoError(t, err)

			require.Equal(t, full.Q.Rows2D(), reduced.Q.Rows2D())
			require.Equal(t, full.R.Rows2D(), reduced.R.Rows2D())

			allocsFull := testing.AllocsPerRun(20, func() {
				if _, err := Decompose(a, ModeFull); err != nil {
					t.Error(err)
				}
			})
			allocsReduced := testing.AllocsPerRun(20, func() {
				if _, err := Decompose(a, ModeReduced); err != nil {
					t.Error(err)
				}
			})
			require.Equal(t, allocsFull, allocsReduced,
				"the reduced mode must not allocate the two submatrix copies")
		})
	}
}

func TestDecompose_Random(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(0x5EED, 0xC0FFEE))
	modes := []Mode{ModeFull, ModeReduced}

	for i := range 500 {
		m := 1 + rng.IntN(8)
		n := 1 + rng.IntN(8)
		scale := math.Pow(10, float64(rng.IntN(61)-30))

		rows := make([][]float64, m)
		for r := range rows {
			rows[r] = make([]float64, n)
			for c := range rows[r] {
				rows[r][c] = (rng.Float64()*2 - 1) * scale
			}
		}
		decompose(t, rows, modes[i%len(modes)])
	}
}

func TestParseMode(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in   string
		want Mode
		ok   bool
	}{
		{"", ModeFull, true},
		{"full", ModeFull, true},
		{"reduced", ModeReduced, true},
		{"thin", "", false},
		{"FULL", "", false},
		{" full", "", false},
	} {
		got, ok := ParseMode(tc.in)
		require.Equal(t, tc.ok, ok, "input %q", tc.in)
		require.Equal(t, tc.want, got, "input %q", tc.in)
	}
}

// hilbert is a classic ill-conditioned case (kappa ~ 5e5 at n = 5).
func hilbert(n int) [][]float64 {
	rows := make([][]float64, n)
	for i := range rows {
		rows[i] = make([]float64, n)
		for j := range rows[i] {
			rows[i][j] = 1 / float64(i+j+1)
		}
	}
	return rows
}

func TestDecompose_NumericalOverflow(t *testing.T) {
	t.Parallel()

	rows := [][]float64{
		{math.MaxFloat64, math.MaxFloat64},
		{math.MaxFloat64, math.MaxFloat64},
		{math.MaxFloat64, math.MaxFloat64},
	}
	_, err := Decompose(matrix.FromRows(rows), ModeFull)
	require.ErrorIs(t, err, ErrNumericalOverflow)
}

func TestDecompose_PrescalingNeverRoundsTheSmallestEntry(t *testing.T) {
	t.Parallel()

	// The largest entry is above the safe band; dividing by 2^24 would push the subnormal corner
	// deeper and cost R[1][1] 4e-7. Diagonal, so any deviation is the prescaling.
	for _, tc := range []struct {
		name string
		rows [][]float64
		want float64
	}{
		{"huge first", [][]float64{{1e308, 0}, {0, 1e-310}}, 1e-310},
		{"tiny first", [][]float64{{1e-310, 0}, {0, 1e308}}, 1e308},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, d := decompose(t, tc.rows, ModeFull)
			require.Equal(t, tc.want, d.R.At(1, 1), "R[1][1] must be bit-for-bit the input entry")
			require.Equal(t, tc.rows[0][0], d.R.At(0, 0), "R[0][0] must be bit-for-bit the input entry")
			require.Zero(t, d.R.At(0, 1))
		})
	}
}

func TestScalingExponent_ClampedByTheSmallestMagnitude(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		minNonZero, maxAbs float64
		want               int
	}{
		{"inside the safe band", 1, 1e10, 0},
		{"all entries huge", 1e307, 1e308, 24},
		{"subnormal corner blocks any shift", 1e-310, 1e308, 0},
		{"small corner allows a partial shift", 1e-305, 1e308, 8},
		{"all entries tiny scale up", 1e-320, 1e-310, -29},
		{"empty or zero matrix", 0, 0, 0},
		{"infinity is left to the finiteness check", 1, math.Inf(1), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, scalingExponent(tc.minNonZero, tc.maxAbs))
		})
	}
}

func TestDecompose_HugeAndTinyStayOrthogonal(t *testing.T) {
	t.Parallel()

	// Rank-deficient over 1e300: trailing columns fall subnormal, where a naive reflector drifts.
	seeds := []float64{1e150, -7e-150, 33, -42}
	rows := make([][]float64, 6)
	for i := range rows {
		rows[i] = make([]float64, 6)
		for j := range rows[i] {
			rows[i][j] = seeds[(i*6+j)%len(seeds)]
		}
	}
	decompose(t, rows, ModeFull)
}
