// Package qr factors A = Q*R by Householder reflections, standard library only.
// [Decompose] guarantees exact zeros below the diagonal, R[i][i] >= 0 (sign ambiguity
// resolved), no negative zero, and [ErrNumericalOverflow] instead of a NaN or Inf.
package qr
