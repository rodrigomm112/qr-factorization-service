package dto_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/dto"
	"github.com/rodrigomm/proyectot/qr-api/internal/domain/matrix"
)

func TestDecodeQrRequest_Valid(t *testing.T) {
	t.Parallel()

	rows, mode, violations, err := dto.DecodeQrRequest([]byte(`{"matrix":[[1,2],[3,4.5],[-5,6e2]],"mode":"reduced"}`))
	require.NoError(t, err)
	require.Empty(t, violations)
	require.Equal(t, [][]float64{{1, 2}, {3, 4.5}, {-5, 600}}, rows)
	require.Equal(t, "reduced", mode)
}

func TestDecodeQrRequest_ModeIsOptional(t *testing.T) {
	t.Parallel()

	_, mode, violations, err := dto.DecodeQrRequest([]byte(`{"matrix":[[1]]}`))
	require.NoError(t, err)
	require.Empty(t, violations)
	require.Empty(t, mode, "an absent mode means the default")
}

func TestDecodeQrRequest_MalformedJSON(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"truncated":      `{"matrix":[[1,2]`,
		"NaN literal":    `{"matrix":[[NaN]]}`,
		"Infinity":       `{"matrix":[[Infinity]]}`,
		"minus Infinity": `{"matrix":[[-Infinity]]}`,
		"empty body":     ``,
		"trailing junk":  `{"matrix":[[1]]} {"matrix":[[2]]}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, _, _, err := dto.DecodeQrRequest([]byte(body))
			require.ErrorIs(t, err, dto.ErrMalformedJSON)
		})
	}
}

// problems.md §3: a wrong root type is a 422 with the whole-document pointer "".
func TestDecodeQrRequest_RootMustBeAnObject(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"array":   `[1,2,3]`,
		"string":  `"x"`,
		"number":  `123`,
		"null":    `null`,
		"boolean": `true`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rows, mode, violations, err := dto.DecodeQrRequest([]byte(body))
			require.NoError(t, err, "the document parses: the type is what is wrong")
			require.Nil(t, rows)
			require.Empty(t, mode)
			require.Equal(t, []matrix.Violation{
				{Pointer: "", Code: matrix.CodeInvalidType, Message: "the request body must be a JSON object"},
			}, violations)
		})
	}
}

func TestDecodeTokenRequest_RootMustBeAnObject(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"array":  `[1,2,3]`,
		"string": `"x"`,
		"number": `123`,
		"null":   `null`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req, violations, err := dto.DecodeTokenRequest([]byte(body))
			require.NoError(t, err)
			require.Nil(t, req.ClientID)
			require.Nil(t, req.ClientSecret)
			require.Equal(t, []matrix.Violation{
				{Pointer: "", Code: matrix.CodeInvalidType, Message: "the request body must be a JSON object"},
			}, violations)
		})
	}
}

func TestDecodeQrRequest_Violations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want []matrix.Violation
	}{
		{
			name: "matrix missing",
			body: `{"mode":"full"}`,
			want: []matrix.Violation{{Pointer: "/matrix", Code: matrix.CodeMissingField, Message: "matrix is required"}},
		},
		{
			name: "matrix null",
			body: `{"matrix":null}`,
			want: []matrix.Violation{{Pointer: "/matrix", Code: matrix.CodeInvalidType, Message: "matrix must be an array of arrays of numbers"}},
		},
		{
			name: "matrix is an object",
			body: `{"matrix":{"rows":1}}`,
			want: []matrix.Violation{{Pointer: "/matrix", Code: matrix.CodeInvalidType, Message: "matrix must be an array of arrays of numbers"}},
		},
		{
			name: "matrix is a number",
			body: `{"matrix":5}`,
			want: []matrix.Violation{{Pointer: "/matrix", Code: matrix.CodeInvalidType, Message: "matrix must be an array of arrays of numbers"}},
		},
		{
			name: "row is not an array",
			body: `{"matrix":[[1,2],7]}`,
			want: []matrix.Violation{{Pointer: "/matrix/1", Code: matrix.CodeInvalidType, Message: "row 1 must be an array of numbers"}},
		},
		{
			// `null` leaves the slice nil; unchecked it would be misreported as empty_row.
			name: "null row is the only row",
			body: `{"matrix":[null]}`,
			want: []matrix.Violation{{Pointer: "/matrix/0", Code: matrix.CodeInvalidType, Message: "row 0 must be an array of numbers"}},
		},
		{
			name: "null row after a valid row",
			body: `{"matrix":[[1,2],null]}`,
			want: []matrix.Violation{{Pointer: "/matrix/1", Code: matrix.CodeInvalidType, Message: "row 1 must be an array of numbers"}},
		},
		{
			name: "null row before a valid row",
			body: `{"matrix":[null,[1,2]]}`,
			want: []matrix.Violation{{Pointer: "/matrix/0", Code: matrix.CodeInvalidType, Message: "row 0 must be an array of numbers"}},
		},
		{
			name: "null cell never becomes zero",
			body: `{"matrix":[[1,null]]}`,
			want: []matrix.Violation{{Pointer: "/matrix/0/1", Code: matrix.CodeInvalidType, Message: "cell [0][1] must be a number"}},
		},
		{
			name: "string cell",
			body: `{"matrix":[["1"]]}`,
			want: []matrix.Violation{{Pointer: "/matrix/0/0", Code: matrix.CodeInvalidType, Message: "cell [0][0] must be a number"}},
		},
		{
			name: "boolean cell",
			body: `{"matrix":[[true]]}`,
			want: []matrix.Violation{{Pointer: "/matrix/0/0", Code: matrix.CodeInvalidType, Message: "cell [0][0] must be a number"}},
		},
		{
			name: "object cell",
			body: `{"matrix":[[{"v":1}]]}`,
			want: []matrix.Violation{{Pointer: "/matrix/0/0", Code: matrix.CodeInvalidType, Message: "cell [0][0] must be a number"}},
		},
		{
			name: "array cell",
			body: `{"matrix":[[[1]]]}`,
			want: []matrix.Violation{{Pointer: "/matrix/0/0", Code: matrix.CodeInvalidType, Message: "cell [0][0] must be a number"}},
		},
		{
			name: "overflowing literal",
			body: `{"matrix":[[1e999]]}`,
			want: []matrix.Violation{{Pointer: "/matrix/0/0", Code: matrix.CodeNonFiniteValue, Message: "cell [0][0] is out of the float64 range"}},
		},
		{
			name: "negative overflowing literal",
			body: `{"matrix":[[-1e999]]}`,
			want: []matrix.Violation{{Pointer: "/matrix/0/0", Code: matrix.CodeNonFiniteValue, Message: "cell [0][0] is out of the float64 range"}},
		},
		{
			name: "mode is not a string",
			body: `{"matrix":[[1]],"mode":7}`,
			want: []matrix.Violation{{Pointer: "/mode", Code: matrix.CodeInvalidType, Message: "mode must be a string"}},
		},
		{
			name: "mode is null",
			body: `{"matrix":[[1]],"mode":null}`,
			want: []matrix.Violation{{Pointer: "/mode", Code: matrix.CodeInvalidType, Message: "mode must be a string"}},
		},
		{
			name: "every bad cell is reported",
			body: `{"matrix":[[null,"a"],[true,1]]}`,
			want: []matrix.Violation{
				{Pointer: "/matrix/0/0", Code: matrix.CodeInvalidType, Message: "cell [0][0] must be a number"},
				{Pointer: "/matrix/0/1", Code: matrix.CodeInvalidType, Message: "cell [0][1] must be a number"},
				{Pointer: "/matrix/1/0", Code: matrix.CodeInvalidType, Message: "cell [1][0] must be a number"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, violations, err := dto.DecodeQrRequest([]byte(tc.body))
			require.NoError(t, err)
			require.Equal(t, tc.want, violations)
		})
	}
}

func TestDecodeQrRequest_UnderflowingLiteralIsZero(t *testing.T) {
	t.Parallel()

	// 1e-999 underflows with ErrRange too, but 0 is a good float64 and must pass.
	rows, _, violations, err := dto.DecodeQrRequest([]byte(`{"matrix":[[1e-999]]}`))
	require.NoError(t, err)
	require.Empty(t, violations)
	require.Equal(t, [][]float64{{0}}, rows)
}

func TestDecodeTokenRequest(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		req, violations, err := dto.DecodeTokenRequest([]byte(`{"clientId":"demo-client","clientSecret":"s3cret"}`))
		require.NoError(t, err)
		require.Empty(t, violations)
		require.Equal(t, "demo-client", *req.ClientID)
		require.Equal(t, "s3cret", *req.ClientSecret)
	})

	t.Run("absent fields stay nil", func(t *testing.T) {
		t.Parallel()
		req, violations, err := dto.DecodeTokenRequest([]byte(`{}`))
		require.NoError(t, err)
		require.Empty(t, violations)
		require.Nil(t, req.ClientID)
		require.Nil(t, req.ClientSecret)
	})

	t.Run("wrong type is a violation, not a syntax error", func(t *testing.T) {
		t.Parallel()
		_, violations, err := dto.DecodeTokenRequest([]byte(`{"clientId":42,"clientSecret":"s"}`))
		require.NoError(t, err)
		require.Equal(t, []matrix.Violation{
			{Pointer: "/clientId", Code: matrix.CodeInvalidType, Message: "clientId must be a string"},
		}, violations)
	})

	t.Run("malformed", func(t *testing.T) {
		t.Parallel()
		_, _, err := dto.DecodeTokenRequest([]byte(`{"clientId":`))
		require.ErrorIs(t, err, dto.ErrMalformedJSON)
	})

	t.Run("trailing junk", func(t *testing.T) {
		t.Parallel()
		_, _, err := dto.DecodeTokenRequest([]byte(`{} {}`))
		require.ErrorIs(t, err, dto.ErrMalformedJSON)
	})
}
