package matrix

// Code is a machine-readable validation failure, shared with stats-api (problems.md §3).
type Code string

const (
	CodeMissingField      Code = "missing_field"
	CodeInvalidType       Code = "invalid_type"
	CodeEmptyMatrix       Code = "empty_matrix"
	CodeEmptyRow          Code = "empty_row"
	CodeRaggedRow         Code = "ragged_row"
	CodeNonFiniteValue    Code = "non_finite_value"
	CodeTooManyRows       Code = "too_many_rows"
	CodeTooManyCols       Code = "too_many_cols"
	CodeInvalidMode       Code = "invalid_mode"
	CodeNumericalOverflow Code = "numerical_overflow"
)

// Violation is a single failed validation rule.
type Violation struct {
	// Pointer is an RFC 6901 JSON Pointer into the request body, e.g. "/matrix/1".
	Pointer string
	Code    Code
	// Message is human readable and never echoes matrix contents.
	Message string
}
