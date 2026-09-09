package matrix

import "math"

// Matrix is an immutable-by-convention float64 matrix in one row-major slice, for locality.
type Matrix struct {
	data []float64
	rows int
	cols int
}

// New returns a zeroed rows x cols matrix; it panics on non-positive dimensions.
func New(rows, cols int) Matrix {
	if rows <= 0 || cols <= 0 {
		panic("matrix: dimensions must be positive")
	}
	return Matrix{data: make([]float64, rows*cols), rows: rows, cols: cols}
}

// Identity returns the n x n identity matrix.
func Identity(n int) Matrix {
	m := New(n, n)
	for i := range n {
		m.data[i*n+i] = 1
	}
	return m
}

// FromRows copies a rectangular [][]float64 into a Matrix; it panics on a ragged input.
func FromRows(rows [][]float64) Matrix {
	if len(rows) == 0 || len(rows[0]) == 0 {
		panic("matrix: FromRows requires a non-empty rectangular input")
	}
	m := New(len(rows), len(rows[0]))
	for i, row := range rows {
		if len(row) != m.cols {
			panic("matrix: FromRows requires a rectangular input")
		}
		copy(m.data[i*m.cols:(i+1)*m.cols], row)
	}
	return m
}

// Rows returns the number of rows.
func (m Matrix) Rows() int { return m.rows }

// Cols returns the number of columns.
func (m Matrix) Cols() int { return m.cols }

// At returns the entry at (i, j).
func (m Matrix) At(i, j int) float64 { return m.data[i*m.cols+j] }

// Set writes the entry at (i, j).
func (m Matrix) Set(i, j int, v float64) { m.data[i*m.cols+j] = v }

// Clone returns a deep copy.
func (m Matrix) Clone() Matrix {
	data := make([]float64, len(m.data))
	copy(data, m.data)
	return Matrix{data: data, rows: m.rows, cols: m.cols}
}

// Rows2D materializes the matrix as a slice of rows, the shape used on the wire.
func (m Matrix) Rows2D() [][]float64 {
	out := make([][]float64, m.rows)
	for i := range m.rows {
		row := make([]float64, m.cols)
		copy(row, m.data[i*m.cols:(i+1)*m.cols])
		out[i] = row
	}
	return out
}

// Submatrix copies the top-left rows x cols block; it cuts the thin factors out.
func (m Matrix) Submatrix(rows, cols int) Matrix {
	out := New(rows, cols)
	for i := range rows {
		copy(out.data[i*cols:(i+1)*cols], m.data[i*m.cols:i*m.cols+cols])
	}
	return out
}

// IsFinite reports whether every entry is neither NaN nor infinite.
func (m Matrix) IsFinite() bool {
	for _, v := range m.data {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// AbsRange returns the smallest non-zero and the largest magnitude in one pass, 0 for zeros.
func (m Matrix) AbsRange() (minNonZero, maxAbs float64) {
	minNonZero = math.Inf(1)
	for _, v := range m.data {
		a := math.Abs(v)
		// NaN compares false both ways and drops out; Validate rejects it long before here.
		if a > maxAbs {
			maxAbs = a
		}
		if a != 0 && a < minNonZero {
			minNonZero = a
		}
	}
	if math.IsInf(minNonZero, 1) {
		minNonZero = 0
	}
	return minNonZero, maxAbs
}

// ScaleByPow2 multiplies every entry by 2^exp, exactly while entries stay normal: below
// 2^-1022 the subnormals drop mantissa bits, so bound exp with [Matrix.AbsRange].
func (m Matrix) ScaleByPow2(exp int) {
	if exp == 0 {
		return
	}
	for i, v := range m.data {
		m.data[i] = math.Ldexp(v, exp)
	}
}

// NormalizeZeros rewrites negative zeros as positive; the JSON encoder never emits "-0".
func (m Matrix) NormalizeZeros() {
	for i, v := range m.data {
		if v == 0 {
			m.data[i] = 0
		}
	}
}
