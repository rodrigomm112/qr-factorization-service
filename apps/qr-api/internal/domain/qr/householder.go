package qr

import (
	"math"

	"github.com/rodrigomm/proyectot/qr-api/internal/domain/matrix"
)

// Mode selects the shape of the returned factors.
type Mode string

const (
	// ModeFull returns Q as m x m and R as m x n; Q is an orthogonal basis of R^m.
	ModeFull Mode = "full"
	// ModeReduced returns the thin factors, k = min(m, n); for m <= n they are the full ones.
	ModeReduced Mode = "reduced"
)

// ParseMode maps a wire value to a Mode; "" means ModeFull, any other value is false.
func ParseMode(s string) (Mode, bool) {
	switch s {
	case "":
		return ModeFull, true
	case string(ModeFull):
		return ModeFull, true
	case string(ModeReduced):
		return ModeReduced, true
	default:
		return "", false
	}
}

// Decomposition is the result of [Decompose]: A = Q*R.
type Decomposition struct {
	Q    matrix.Matrix
	R    matrix.Matrix
	Mode Mode
}

// safeExponent: inside +-1000 every reflection intermediate stays 300 orders from the edges.
const safeExponent = 1000

// minNormalExponent: below frexp's -1021 (2^-1022) entries go subnormal and lose bits.
const minNormalExponent = -1021

// scalingExponent is the power of two to divide A by, clamped by minNonZero so it stays exact.
func scalingExponent(minNonZero, maxAbs float64) int {
	if maxAbs == 0 || math.IsInf(maxAbs, 0) || math.IsNaN(maxAbs) {
		return 0
	}
	_, expMax := math.Frexp(maxAbs)
	switch {
	case expMax > safeExponent:
		_, expMin := math.Frexp(minNonZero)
		return max(0, min(expMax-safeExponent, expMin-minNormalExponent))
	case expMax < -safeExponent:
		return expMax + safeExponent
	default:
		return 0
	}
}

// Decompose factors a as Q*R over the min(n, m-1) leading columns. Sign chosen away from
// cancellation: alpha = -sign(sub[0])*||sub||, so v[0] = sub[0] - alpha never cancels.
func Decompose(a matrix.Matrix, mode Mode) (Decomposition, error) {
	m, n := a.Rows(), a.Cols()

	r := a.Clone()
	q := matrix.Identity(m)

	// Prescale by a power of two: entries near MaxFloat64/4 otherwise overflow and wreck Q.
	exponent := scalingExponent(r.AbsRange())
	r.ScaleByPow2(-exponent)

	v := make([]float64, m) // reused across steps; only v[:m-k] is live
	steps := min(n, m-1)
	for k := range steps {
		sub := v[:m-k]
		for i := k; i < m; i++ {
			sub[i-k] = r.At(i, k)
		}

		// A reflector's direction survives positive rescaling; normalize to [0.5, 1) so a
		// rank-deficient column does not decide v/||v|| with subnormal mantissa bits.
		columnExponent := 0
		if maxAbs := maxAbsSlice(sub); maxAbs > 0 {
			_, columnExponent = math.Frexp(maxAbs)
			scalePow2(sub, -columnExponent)
		}

		alpha := scaledNorm2(sub)
		if alpha == 0 {
			// Zero sub-column: nothing to reflect, and rank deficiency is a legal input.
			continue
		}
		if sub[0] >= 0 {
			alpha = -alpha
		}
		sub[0] -= alpha

		// norm > 0: the rescale put max|sub| in [0.5, 1), so |alpha| >= 0.5, and subtracting
		// an alpha of the opposite sign only grows |sub[0]| past 0.5.
		norm := scaledNorm2(sub)
		for i := range sub {
			sub[i] /= norm
		}

		// R[k:, k:] -= 2*v*(v^T * R[k:, k:])
		for j := k; j < n; j++ {
			dot := 0.0
			for i := k; i < m; i++ {
				dot += sub[i-k] * r.At(i, j)
			}
			dot *= 2
			for i := k; i < m; i++ {
				r.Set(i, j, r.At(i, j)-dot*sub[i-k])
			}
		}
		// The reflected column is alpha*e_1 exactly; write it, undoing the per-column rescale.
		r.Set(k, k, math.Ldexp(alpha, columnExponent))
		for i := k + 1; i < m; i++ {
			r.Set(i, k, 0)
		}

		// Q <- Q*H_k, i.e. Q[:, k:] -= 2*(Q[:, k:] * v)*v^T
		for i := range m {
			dot := 0.0
			for j := k; j < m; j++ {
				dot += q.At(i, j) * sub[j-k]
			}
			dot *= 2
			for j := k; j < m; j++ {
				q.Set(i, j, q.At(i, j)-dot*sub[j-k])
			}
		}
	}

	// The prescale multiplied A, so undoing it on R alone restores A = Q*R.
	r.ScaleByPow2(exponent)

	// No zeroing pass: step k wrote alpha*e_1 into column k, and a pow2 rescale keeps zeros.

	// A = (Q*D)*(D*R) for diagonal D of +-1; D chosen so R[i][i] >= 0 makes the result unique.
	for i := range min(m, n) {
		if r.At(i, i) < 0 {
			for j := range n {
				r.Set(i, j, -r.At(i, j))
			}
			for row := range m {
				q.Set(row, i, -q.At(row, i))
			}
		}
	}

	// For k == m -- square and wide -- the submatrices would be exact copies, so skip them.
	if k := min(m, n); mode == ModeReduced && k < m {
		q = q.Submatrix(m, k)
		r = r.Submatrix(k, n)
	}

	q.NormalizeZeros()
	r.NormalizeZeros()

	if !q.IsFinite() || !r.IsFinite() {
		return Decomposition{}, ErrNumericalOverflow
	}
	return Decomposition{Q: q, R: r, Mode: mode}, nil
}
