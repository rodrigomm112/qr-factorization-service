package qr

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/rodrigomm/proyectot/qr-api/internal/domain/matrix"
)

// benchMatrix builds a deterministic well-scaled m x n matrix, so prescaling is a no-op.
func benchMatrix(m, n int) matrix.Matrix {
	random := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // deterministic input, not a security decision
	a := matrix.New(m, n)
	for i := range m {
		for j := range n {
			a.Set(i, j, random.Float64()*2-1)
		}
	}
	return a
}

// BenchmarkDecompose covers the accepted sizes (100x100 per ADR-0014, 100x10 for the tall
// fixture). Forming Q explicitly adds O(m^2*min(m, n)) on top of the ~2mn^2 - 2n^3/3.
func BenchmarkDecompose(b *testing.B) {
	sizes := []struct{ m, n int }{
		{10, 10},
		{50, 50},
		{100, 100},
		{100, 10},
	}
	modes := []Mode{ModeFull, ModeReduced}

	for _, size := range sizes {
		a := benchMatrix(size.m, size.n)
		for _, mode := range modes {
			name := strconv.Itoa(size.m) + "x" + strconv.Itoa(size.n) + "/" + string(mode)
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := Decompose(a, mode); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkScaledNorm2(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		v := make([]float64, n)
		random := rand.New(rand.NewPCG(3, 4)) //nolint:gosec // deterministic input
		for i := range v {
			v[i] = random.Float64()*2 - 1
		}
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			var sink float64
			for b.Loop() {
				sink = scaledNorm2(v)
			}
			_ = sink
		})
	}
}
