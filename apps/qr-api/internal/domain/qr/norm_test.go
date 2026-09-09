package qr

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScaledNorm2(t *testing.T) {
	t.Parallel()

	const eps = 1e-15

	t.Run("classic triple", func(t *testing.T) {
		t.Parallel()
		require.InDelta(t, 5.0, scaledNorm2([]float64{3, 4}), eps)
	})

	t.Run("empty and zero vectors are exactly zero", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, 0.0, scaledNorm2(nil))
		require.Equal(t, 0.0, scaledNorm2([]float64{}))
		require.Equal(t, 0.0, scaledNorm2([]float64{0, 0, 0}))
		require.Equal(t, 0.0, scaledNorm2([]float64{0, math.Copysign(0, -1)}))
	})

	t.Run("single entry is its magnitude", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, 3.5, scaledNorm2([]float64{-3.5}))
	})

	t.Run("no overflow near the top of the range", func(t *testing.T) {
		t.Parallel()
		// The naive sqrt(sum x^2) overflows here: (1e200)^2 is +Inf.
		got := scaledNorm2([]float64{1e200, 1e200})
		require.InEpsilon(t, 1e200*math.Sqrt2, got, 1e-12)
		require.False(t, math.IsInf(got, 0))
	})

	t.Run("no underflow near the bottom of the range", func(t *testing.T) {
		t.Parallel()
		// The naive form flushes (1e-200)^2 to zero and returns 0.
		got := scaledNorm2([]float64{1e-200, 1e-200})
		require.InEpsilon(t, 1e-200*math.Sqrt2, got, 1e-12)
		require.Positive(t, got)
	})

	t.Run("mixed magnitudes stay accurate", func(t *testing.T) {
		t.Parallel()
		require.InEpsilon(t, 1e150, scaledNorm2([]float64{1e150, 1e-150, 1}), 1e-12)
	})

	t.Run("propagates non-finite inputs", func(t *testing.T) {
		t.Parallel()
		require.True(t, math.IsNaN(scaledNorm2([]float64{1, math.NaN()})))
		require.True(t, math.IsInf(scaledNorm2([]float64{1, math.Inf(1)}), 1))
		require.True(t, math.IsInf(scaledNorm2([]float64{1, math.Inf(-1)}), 1))
	})
}
