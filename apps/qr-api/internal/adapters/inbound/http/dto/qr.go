// Package dto decodes qr-api bodies, reporting each bad cell as a [matrix.Violation].
package dto

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/rodrigomm/proyectot/qr-api/internal/domain/matrix"
)

// QrRequest is the body of POST /api/v1/qr. Both members are values, not pointers, so
// UnmarshalJSON still runs on a literal `null` and an explicit null differs from an absent one.
type QrRequest struct {
	Matrix MatrixDTO `json:"matrix"`
	Mode   ModeDTO   `json:"mode"`
}

// ModeDTO decodes `mode`, turning a non-string into invalid_type rather than a parse error.
type ModeDTO struct {
	Value string
	// Present is true as soon as the member appears in the body, even as null.
	Present bool
	// WrongType covers null and any non-string value.
	WrongType bool
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *ModeDTO) UnmarshalJSON(data []byte) error {
	m.Present = true
	var s string
	// encoding/json takes `null` into a string as a no-op; `null` is not an allowed mode.
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		m.WrongType = true
		return nil
	}
	if err := json.Unmarshal(data, &s); err != nil {
		m.WrongType = true
		return nil
	}
	m.Value = s
	return nil
}

// MatrixDTO is a decoded matrix plus the violations found while decoding it.
type MatrixDTO struct {
	Rows       [][]float64
	Violations []matrix.Violation
	// Present is true as soon as the member appears in the body, even as null.
	Present bool
}

// UnmarshalJSON decodes arrays of finite JSON numbers, accumulating type problems as issues.
func (m *MatrixDTO) UnmarshalJSON(data []byte) error {
	m.Rows = nil
	m.Violations = nil
	m.Present = true

	var outer []json.RawMessage
	if err := json.Unmarshal(data, &outer); err != nil || outer == nil {
		m.Violations = append(m.Violations, matrix.Violation{
			Pointer: pointerMatrix,
			Code:    matrix.CodeInvalidType,
			Message: "matrix must be an array of arrays of numbers",
		})
		return nil
	}

	m.Rows = make([][]float64, len(outer))
	for i, rawRow := range outer {
		rowPointer := pointerMatrix + "/" + strconv.Itoa(i)
		var inner []json.RawMessage
		// `null` decodes to a nil slice: unchecked it would read as empty_row, not invalid_type.
		if err := json.Unmarshal(rawRow, &inner); err != nil || inner == nil {
			m.Violations = append(m.Violations, matrix.Violation{
				Pointer: rowPointer,
				Code:    matrix.CodeInvalidType,
				Message: fmt.Sprintf("row %d must be an array of numbers", i),
			})
			m.Rows[i] = nil
			continue
		}
		row := make([]float64, len(inner))
		for j, rawCell := range inner {
			cellPointer := rowPointer + "/" + strconv.Itoa(j)
			value, violation := decodeCell(rawCell, cellPointer, i, j)
			if violation != nil {
				m.Violations = append(m.Violations, *violation)
				continue
			}
			row[j] = value
		}
		m.Rows[i] = row
	}
	return nil
}

const pointerMatrix = "/matrix"

// decodeCell takes only a finite JSON number; +-Inf from a 1e999 literal is non_finite_value.
func decodeCell(raw json.RawMessage, pointer string, i, j int) (float64, *matrix.Violation) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || !isJSONNumberStart(trimmed[0]) {
		return 0, &matrix.Violation{
			Pointer: pointer,
			Code:    matrix.CodeInvalidType,
			Message: fmt.Sprintf("cell [%d][%d] must be a number", i, j),
		}
	}
	value, err := strconv.ParseFloat(string(trimmed), 64)
	if err != nil {
		var numErr *strconv.NumError
		if errors.As(err, &numErr) && numErr.Err == strconv.ErrRange {
			// 1e999 parses to +Inf with ErrRange: valid JSON, no float64 to hold it.
			return 0, &matrix.Violation{
				Pointer: pointer,
				Code:    matrix.CodeNonFiniteValue,
				Message: fmt.Sprintf("cell [%d][%d] is out of the float64 range", i, j),
			}
		}
		return 0, &matrix.Violation{
			Pointer: pointer,
			Code:    matrix.CodeInvalidType,
			Message: fmt.Sprintf("cell [%d][%d] must be a number", i, j),
		}
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, &matrix.Violation{
			Pointer: pointer,
			Code:    matrix.CodeNonFiniteValue,
			Message: fmt.Sprintf("cell [%d][%d] is not a finite number", i, j),
		}
	}
	return value, nil
}

func isJSONNumberStart(b byte) bool {
	return b == '-' || (b >= '0' && b <= '9')
}

// TokenRequest is the body of POST /api/v1/auth/token.
type TokenRequest struct {
	ClientID     *string `json:"clientId"`
	ClientSecret *string `json:"clientSecret"`
}
