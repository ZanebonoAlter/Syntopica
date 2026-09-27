package datasources

import (
	"errors"
	"fmt"
)

// ErrorKind is the unified error taxonomy for all data sources (spec
// "统一错误语义"). Three kinds only; regular no-data results are NOT errors.
type ErrorKind string

const (
	// ErrInvalidArgument: caller-supplied parameters are illegal. The source
	// MUST NOT have issued any network request when this is returned.
	ErrInvalidArgument ErrorKind = "INVALID_ARGUMENT"
	// ErrSourceUnavailable: network failure, timeout, size/time budget
	// exceeded, upstream 5xx, or required configuration (e.g. API key) missing.
	ErrSourceUnavailable ErrorKind = "SOURCE_UNAVAILABLE"
	// ErrSchemaChanged: upstream structure drifted (column layout, unknown
	// value markers, duplicate dimension conflicts). No fuzzy matching.
	ErrSchemaChanged ErrorKind = "SCHEMA_CHANGED"
)

// SourceError is the typed error every fetcher returns. Kind drives the HTTP
// status mapping (400/502) and the cache eviction policy (SCHEMA_CHANGED
// evicts the cached entry for the same key).
type SourceError struct {
	Kind    ErrorKind
	Source  string // catalog code, e.g. "eia_wpsr"
	Message string
	Detail  string // optional context (URL, config key name, offending marker)

	// StatusCode carries the upstream HTTP status when the error came from an
	// HTTP response (0 otherwise). Sources use it to distinguish e.g. 404
	// (JODI year fallback) from other failures.
	StatusCode int
}

func (e *SourceError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("[%s] %s: %s (%s)", e.Kind, e.Source, e.Message, e.Detail)
	}
	return fmt.Sprintf("[%s] %s: %s", e.Kind, e.Source, e.Message)
}

func newErr(kind ErrorKind, source, message string) *SourceError {
	return &SourceError{Kind: kind, Source: source, Message: message}
}

func newErrDetail(kind ErrorKind, source, message, detail string) *SourceError {
	return &SourceError{Kind: kind, Source: source, Message: message, Detail: detail}
}

// InvalidArg returns INVALID_ARGUMENT; the caller is expected to validate
// parameters BEFORE any network call.
func InvalidArg(source, message string) *SourceError {
	return newErr(ErrInvalidArgument, source, message)
}

// Unavailable returns SOURCE_UNAVAILABLE.
func Unavailable(source, message string) *SourceError {
	return newErr(ErrSourceUnavailable, source, message)
}

// UnavailableDetail returns SOURCE_UNAVAILABLE with extra detail.
func UnavailableDetail(source, message, detail string) *SourceError {
	return newErrDetail(ErrSourceUnavailable, source, message, detail)
}

// UnavailableStatus returns SOURCE_UNAVAILABLE carrying the upstream HTTP
// status code (fetch layer 404/5xx mapping).
func UnavailableStatus(source string, status int) *SourceError {
	e := newErrDetail(ErrSourceUnavailable, source, fmt.Sprintf("上游返回非 200：status=%d", status), "")
	e.StatusCode = status
	return e
}

// SchemaChanged returns SCHEMA_CHANGED; the corresponding cache entry MUST be
// evicted by the caller.
func SchemaChanged(source, message string) *SourceError {
	return newErr(ErrSchemaChanged, source, message)
}

// SchemaChangedDetail returns SCHEMA_CHANGED with extra detail (offending
// marker, row, column).
func SchemaChangedDetail(source, message, detail string) *SourceError {
	return newErrDetail(ErrSchemaChanged, source, message, detail)
}

// AsSourceError unwraps a *SourceError from err.
func AsSourceError(err error) (*SourceError, bool) {
	var se *SourceError
	if errors.As(err, &se) {
		return se, true
	}
	return nil, false
}
