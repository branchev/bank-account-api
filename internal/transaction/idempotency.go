package transaction

import (
	"errors"
	"net/http"
)

// ErrIdempotencyConflict indicates that an idempotency key
// has already been used with a different request.
var ErrIdempotencyConflict = errors.New(
	"idempotency key already used with a different request",
)

// Result represents the outcome of an idempotency check.
type Result struct {
	// Replay indicates that the operation was previously completed.
	Replay bool

	// StatusCode is the HTTP status to return for a replay.
	StatusCode int

	// Body contains the previously stored JSON response.
	Body []byte
}

// ValidateReplay checks whether an existing transaction
// can be safely replayed for the incoming request.
func ValidateReplay(
	existing *Transaction,
	requestType Type,
	requestHash string,
) (Result, error) {
	if existing == nil {
		return Result{}, errors.New("existing transaction cannot be nil")
	}

	if existing.Type != requestType ||
		existing.RequestHash != requestHash {
		return Result{}, ErrIdempotencyConflict
	}

	if existing.ResponseStatus == nil || existing.ResponseBody == nil {
		return Result{}, errors.New(
			"existing transaction has no completed response",
		)
	}

	return Result{
		Replay:     true,
		StatusCode: *existing.ResponseStatus,
		Body:       existing.ResponseBody,
	}, nil
}

// ConflictStatusCode returns the HTTP status for an
// idempotency conflict.
func ConflictStatusCode() int {
	return http.StatusConflict
}
