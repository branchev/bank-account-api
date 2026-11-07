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
	// Create inserts a new transaction record.
	// The database enforces uniqueness of the idempotency key.
	Create(
		ctx context.Context,
		tx pgx.Tx,
		transaction *Transaction,
	) error

	// GetByIdempotencyKey retrieves a transaction by its unique key.
	// Returns pgx.ErrNoRows if the key does not exist.
	GetByIdempotencyKey(
		ctx context.Context,
		tx pgx.Tx,
		key string,
	) (*Transaction, error)

	// Complete persists the HTTP response after the financial operation
	// has succeeded, before committing the database transaction.
	Complete(
		ctx context.Context,
		tx pgx.Tx,
		id int64,
		responseStatus int,
		responseBody []byte,
	) error
}
