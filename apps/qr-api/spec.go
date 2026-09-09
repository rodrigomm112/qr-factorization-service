// Package qrapi is the module root; //go:embed cannot reach openapi.yaml from a subpackage.
package qrapi

import _ "embed"

// OpenAPISpec is the served openapi.yaml, the same file `redocly lint` checks.
//
//go:embed openapi.yaml
var OpenAPISpec []byte
