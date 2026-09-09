package dto

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rodrigomm/proyectot/qr-api/internal/domain/matrix"
)

// ErrMalformedJSON covers a body that is not valid JSON, NaN/Infinity included. -> 400.
var ErrMalformedJSON = errors.New("dto: malformed JSON body")

// rootPointer is RFC 6901's whole-document pointer, used when the body's root is wrong.
const rootPointer = ""

// rootNotAnObject is the violation for a body that parses but is not an object.
// problems.md §3 puts it on the validation side: valid JSON, wrong type, so 422 and not 400.
func rootNotAnObject() []matrix.Violation {
	return []matrix.Violation{{
		Pointer: rootPointer,
		Code:    matrix.CodeInvalidType,
		Message: "the request body must be a JSON object",
	}}
}

// decodeRoot validates the shared envelope: one valid JSON document (400), an object (422).
func decodeRoot(body []byte) (json.RawMessage, []matrix.Violation, error) {
	var raw json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&raw); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrMalformedJSON, err)
	}
	if err := ensureSingleValue(decoder); err != nil {
		return nil, nil, err
	}
	// The scanner accepted the whole document, so the first non-space byte fixes the root type.
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, rootNotAnObject(), nil
	}
	return raw, nil, nil
}

// DecodeQrRequest parses POST /api/v1/qr; only a syntactically invalid document errors.
func DecodeQrRequest(body []byte) (rows [][]float64, mode string, violations []matrix.Violation, err error) {
	raw, rootViolations, err := decodeRoot(body)
	if err != nil {
		return nil, "", nil, err
	}
	if len(rootViolations) > 0 {
		return nil, "", rootViolations, nil
	}

	var req QrRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		// Defensive: both members accumulate type problems as violations, and the body parses.
		return nil, "", nil, fmt.Errorf("%w: %w", ErrMalformedJSON, err)
	}

	switch {
	case !req.Matrix.Present:
		violations = append(violations, matrix.Violation{
			Pointer: pointerMatrix,
			Code:    matrix.CodeMissingField,
			Message: "matrix is required",
		})
	default:
		rows = req.Matrix.Rows
		violations = append(violations, req.Matrix.Violations...)
	}

	switch {
	case !req.Mode.Present:
		// Absent: the use case applies the default.
	case req.Mode.WrongType:
		violations = append(violations, matrix.Violation{
			Pointer: "/mode",
			Code:    matrix.CodeInvalidType,
			Message: "mode must be a string",
		})
	default:
		mode = req.Mode.Value
	}
	return rows, mode, violations, nil
}

// DecodeTokenRequest parses the body of POST /api/v1/auth/token.
func DecodeTokenRequest(body []byte) (TokenRequest, []matrix.Violation, error) {
	var req TokenRequest
	raw, rootViolations, err := decodeRoot(body)
	if err != nil {
		return req, nil, err
	}
	if len(rootViolations) > 0 {
		return req, rootViolations, nil
	}

	if err := json.Unmarshal(raw, &req); err != nil {
		// A wrong JSON type on a declared field is a validation problem, not a syntax one.
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			return req, []matrix.Violation{{
				Pointer: "/" + typeErr.Field,
				Code:    matrix.CodeInvalidType,
				Message: typeErr.Field + " must be a string",
			}}, nil
		}
		return req, nil, fmt.Errorf("%w: %w", ErrMalformedJSON, err)
	}
	return req, nil, nil
}

// ensureSingleValue rejects trailing content the streaming decoder would otherwise ignore.
func ensureSingleValue(decoder *json.Decoder) error {
	if decoder.More() {
		return fmt.Errorf("%w: unexpected trailing content", ErrMalformedJSON)
	}
	return nil
}
