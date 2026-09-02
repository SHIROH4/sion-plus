// Package errors defines the structured error types used across the project.
//
// Design rules:
//  1. Sentinel errors for well-known failure modes (callers use errors.Is).
//  2. RetryableError wraps transient failures with a suggested backoff duration.
//  3. Adapter layer returns these; application layer decides whether to degrade.
//  4. Never panic — always return an error from this package.
package errors

import (
	"fmt"
	"time"
)

// ── Sentinel Errors ──

var (
	// ErrNotReady is returned when a service hasn't finished initialization.
	ErrNotReady = fmt.Errorf("service not ready")

	// ErrLLMUnavailable is returned when no LLM provider is reachable.
	ErrLLMUnavailable = fmt.Errorf("llm unavailable: all providers failed")

	// ErrQuotaExhausted is returned when the daily proactive action limit is reached.
	ErrQuotaExhausted = fmt.Errorf("daily proactive quota exhausted")

	// ErrConfigInvalid is returned when the configuration fails validation.
	ErrConfigInvalid = fmt.Errorf("configuration is invalid")

	// ErrStorageLocked is returned when another instance holds the data directory lock.
	ErrStorageLocked = fmt.Errorf("storage locked by another instance")

	// ErrStorageNotReady is returned when the database hasn't been initialized.
	ErrStorageNotReady = fmt.Errorf("storage not initialized")

	// ErrEmbeddingUnavailable is returned when the embedding service is down.
	ErrEmbeddingUnavailable = fmt.Errorf("embedding service unavailable")

	// ErrRateLimited is returned when the LLM provider returns 429.
	ErrRateLimited = fmt.Errorf("llm rate limited")

	// ErrTimeout is returned when an operation exceeds its deadline.
	ErrTimeout = fmt.Errorf("operation timed out")

	// ErrShuttingDown is returned when the server is gracefully shutting down.
	ErrShuttingDown = fmt.Errorf("server is shutting down")

	// ErrNotImplemented is returned when a feature is planned but not yet built.
	ErrNotImplemented = fmt.Errorf("not implemented yet")
)

// ── Structured Error Types ──

// RetryableError wraps an error that should be retried after the given delay.
// Callers check with errors.As(err, &RetryableError{}).
type RetryableError struct {
	Err   error
	After time.Duration
}

func (e *RetryableError) Error() string {
	return fmt.Sprintf("%s (retry after %s)", e.Err, e.After)
}

func (e *RetryableError) Unwrap() error { return e.Err }

// NewRetryable creates a retryable error.
func NewRetryable(err error, after time.Duration) *RetryableError {
	return &RetryableError{Err: err, After: after}
}

// DegradedError indicates the operation succeeded but with reduced quality.
// Application layer logs these at Warn level rather than Error.
type DegradedError struct {
	Err    error
	Reason string // "llm_unavailable_used_cache" | "vector_unavailable_used_keyword" | "ocr_failed_used_title"
}

func (e *DegradedError) Error() string {
	return fmt.Sprintf("degraded (%s): %s", e.Reason, e.Err)
}

func (e *DegradedError) Unwrap() error { return e.Err }

// NewDegraded creates a degraded-mode error.
func NewDegraded(err error, reason string) *DegradedError {
	return &DegradedError{Err: err, Reason: reason}
}

// ValidationError carries a list of config validation failures.
type ValidationError struct {
	Fields []ValidationFailure
}

type ValidationFailure struct {
	Field   string
	Value   any
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("config validation failed: %d field(s)", len(e.Fields))
}
