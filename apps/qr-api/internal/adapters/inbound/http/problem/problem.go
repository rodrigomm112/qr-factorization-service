// Package problem implements the RFC 9457 contract shared with stats-api; titles fixed per slug.
package problem

import (
	"fmt"
	"time"
)

// URNPrefix is the base of every problem type.
const URNPrefix = "urn:proyectot:problem:"

// Slug identifies a problem type.
type Slug string

// The shared error registry (same slugs and titles as stats-api).
const (
	SlugMalformedJSON               Slug = "malformed-json"
	SlugUnauthorized                Slug = "unauthorized"
	SlugNotFound                    Slug = "not-found"
	SlugMethodNotAllowed            Slug = "method-not-allowed"
	SlugPayloadTooLarge             Slug = "payload-too-large"
	SlugRequestHeaderFieldsTooLarge Slug = "request-header-fields-too-large"
	SlugUnsupportedMediaType        Slug = "unsupported-media-type"
	SlugValidationError             Slug = "validation-error"
	SlugTooManyRequests             Slug = "too-many-requests"
	SlugInternalError               Slug = "internal-error"
	SlugDownstreamUnavailable       Slug = "downstream-unavailable"
	SlugDownstreamTimeout           Slug = "downstream-timeout"
)

// registry maps each slug to its status and its exact title; clients branch on type, not title.
var registry = map[Slug]struct {
	Title  string
	Status int
}{
	SlugMalformedJSON:               {"Malformed JSON body", 400},
	SlugUnauthorized:                {"Authentication required", 401},
	SlugNotFound:                    {"Resource not found", 404},
	SlugMethodNotAllowed:            {"Method not allowed", 405},
	SlugPayloadTooLarge:             {"Payload too large", 413},
	SlugRequestHeaderFieldsTooLarge: {"Request header fields too large", 431},
	SlugUnsupportedMediaType:        {"Unsupported media type", 415},
	SlugValidationError:             {"The request body failed validation", 422},
	SlugTooManyRequests:             {"Too many requests", 429},
	SlugInternalError:               {"Internal server error", 500},
	SlugDownstreamUnavailable:       {"Upstream dependency unavailable", 502},
	SlugDownstreamTimeout:           {"Upstream dependency timed out", 504},
}

// Title returns the exact title registered for a slug.
func Title(s Slug) string { return registry[s].Title }

// Status returns the HTTP status registered for a slug.
func Status(s Slug) int { return registry[s].Status }

// Issue is one entry of `errors[]` on a validation problem.
type Issue struct {
	Pointer string `json:"pointer"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Document is the serialized problem, in the exact shape of the contract.
type Document struct {
	Type      string  `json:"type"`
	Title     string  `json:"title"`
	Status    int     `json:"status"`
	Detail    string  `json:"detail,omitempty"`
	Instance  string  `json:"instance"`
	RequestID string  `json:"requestId"`
	Timestamp string  `json:"timestamp"`
	Errors    []Issue `json:"errors,omitempty"`
}

// Error is the in-flight problem: handlers return it, the Fiber error handler renders it.
type Error struct {
	Slug   Slug
	Detail string
	Issues []Issue
	// TokenPresented adds error="invalid_token"; RFC 6750 §3 allows it only for a sent token.
	TokenPresented bool
	// Retry-After value in seconds, set on 429.
	RetryAfterSeconds int
	// cause is kept for the log only; it never reaches the client.
	cause error
}

func (e *Error) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s", e.Slug, e.Detail)
	}
	return string(e.Slug)
}

// Unwrap exposes the original error to errors.Is/As and to the logger.
func (e *Error) Unwrap() error { return e.cause }

// WithTokenPresented marks the 401 as a rejected token; it changes WWW-Authenticate only.
func (e *Error) WithTokenPresented() *Error {
	e.TokenPresented = true
	return e
}

// Status returns the HTTP status of the problem.
func (e *Error) Status() int { return Status(e.Slug) }

// Document renders the problem for one request.
func (e *Error) Document(instance, requestID string, now time.Time) Document {
	return Document{
		Type:      URNPrefix + string(e.Slug),
		Title:     Title(e.Slug),
		Status:    Status(e.Slug),
		Detail:    e.Detail,
		Instance:  instance,
		RequestID: requestID,
		Timestamp: now.UTC().Format("2006-01-02T15:04:05.000Z"),
		Errors:    e.Issues,
	}
}

// New builds a problem with the default detail of its slug.
func New(slug Slug, detail string) *Error { return &Error{Slug: slug, Detail: detail} }

// Wrap builds a problem that keeps the underlying cause for the log.
func Wrap(slug Slug, detail string, cause error) *Error {
	return &Error{Slug: slug, Detail: detail, cause: cause}
}

// Constructors for the contract's fixed documents; details match contracts/examples.

// MalformedJSON is the 400 of an unparseable body.
func MalformedJSON(cause error) *Error {
	return Wrap(SlugMalformedJSON, "The request body is not valid JSON.", cause)
}

// Unauthorized is the single 401 document; tokenPresented changes the header, not the body.
func Unauthorized(tokenPresented bool) *Error {
	return &Error{
		Slug:           SlugUnauthorized,
		Detail:         "The access token is missing, expired or invalid.",
		TokenPresented: tokenPresented,
	}
}

// NotFound is the 404 of an unknown path.
func NotFound() *Error {
	return New(SlugNotFound, "The requested resource does not exist.")
}

// MethodNotAllowed is the 405 of a known path with the wrong method.
func MethodNotAllowed() *Error {
	return New(SlugMethodNotAllowed, "This method is not allowed on the requested resource.")
}

// PayloadTooLarge is the 413 of a body above MAX_BODY_BYTES.
func PayloadTooLarge(limit int) *Error {
	return New(SlugPayloadTooLarge, fmt.Sprintf("The request body exceeds the %d byte limit.", limit))
}

// RequestHeaderFieldsTooLarge is the 431 of a request line or header block above the buffer.
func RequestHeaderFieldsTooLarge() *Error {
	return New(SlugRequestHeaderFieldsTooLarge, "The request headers exceed the server's buffer.")
}

// UnsupportedMediaType is the 415 of a non-JSON body.
func UnsupportedMediaType() *Error {
	return New(SlugUnsupportedMediaType, "Content-Type must be application/json.")
}

// Validation is the 422 carrying every failed rule.
func Validation(detail string, issues []Issue) *Error {
	return &Error{Slug: SlugValidationError, Detail: detail, Issues: issues}
}

// TooManyRequests is the 429 of the rate limiter.
func TooManyRequests(retryAfterSeconds int) *Error {
	return &Error{
		Slug:              SlugTooManyRequests,
		Detail:            "Rate limit exceeded. Retry after the window resets.",
		RetryAfterSeconds: retryAfterSeconds,
	}
}

// Internal is the 500: no details ever leave the process.
func Internal(cause error) *Error {
	return Wrap(SlugInternalError, "An unexpected error occurred. Quote the requestId when reporting it.", cause)
}

// DownstreamUnavailable is the 502 of a failed call to stats-api.
func DownstreamUnavailable(cause error) *Error {
	return Wrap(SlugDownstreamUnavailable, "The statistics service did not return a usable response.", cause)
}

// DownstreamTimeout is the 504 of a call to stats-api that ran out of time.
func DownstreamTimeout(cause error) *Error {
	return Wrap(SlugDownstreamTimeout, "The statistics service did not respond in time.", cause)
}
