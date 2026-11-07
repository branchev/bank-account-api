package transaction

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Repository defines persistence operations for financial transactions.
//
// All operations use the caller's PostgreSQL transaction to ensure
// atomicity with account balance updates and ledger entries.
type Repository interface {
	// TryCreate attempts to reserve an idempotency key.
	//
	// created is true when a new transaction was inserted.
	// created is false when the key already exists.
	//
	// PostgreSQL's unique constraint coordinates concurrent requests.
	TryCreate(
		ctx context.Context,
		tx pgx.Tx,
		transaction *Transaction,
	) (created bool, err error)

	// Create inserts a new transaction record.
	// A duplicate idempotency key returns a database error.
	Create(
		ctx context.Context,
		tx pgx.Tx,
		transaction *Transaction,
	) error

	// GetByIdempotencyKey retrieves an existing transaction.
	// Returns pgx.ErrNoRows if the key does not exist.
	GetByIdempotencyKey(
		ctx context.Context,
		tx pgx.Tx,
		key string,
	) (*Transaction, error)

	// Complete stores the HTTP response before the caller commits.
	Complete(
		ctx context.Context,
		tx pgx.Tx,
		id int64,
		responseStatus int,
		responseBody []byte,
	) error
}
