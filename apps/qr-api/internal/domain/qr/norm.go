package qr

import "math"

// scaledNorm2 is LAPACK's dnrm2: rescaling around the running maximum stays accurate over
// the whole float64 range, where naive sqrt(sum x^2) overflows or flushes to zero.
func scaledNorm2(v []float64) float64 {
	scale, ssq := 0.0, 1.0
	for _, x := range v {
		if x == 0 {
			continue
		}
		ax := math.Abs(x)
		if math.IsNaN(ax) {
			return math.NaN()
		}
		if math.IsInf(ax, 0) {
			return math.Inf(1)
		}
		if scale < ax {
			r := scale / ax
			ssq = 1 + ssq*r*r
			scale = ax
		} else {
			r := ax / scale
			ssq += r * r
		}
	}
	return scale * math.Sqrt(ssq)
}

func maxAbsSlice(v []float64) float64 {
	maxAbs := 0.0
	for _, x := range v {
		if a := math.Abs(x); a > maxAbs {
			maxAbs = a
		}
	}
	return maxAbs
}

// scalePow2 scales v by 2^exp; exact, no rounding.
func scalePow2(v []float64, exp int) {
	if exp == 0 {
		return
	}
	for i, x := range v {
		v[i] = math.Ldexp(x, exp)
	}
}
