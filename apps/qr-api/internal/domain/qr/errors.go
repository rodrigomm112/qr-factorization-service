package qr

import "errors"

// ErrNumericalOverflow reports a non-finite entry from finite input. -> 422, never 500.
var ErrNumericalOverflow = errors.New("qr: decomposition produced a non-finite entry")
