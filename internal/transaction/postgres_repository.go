package transaction

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// PostgresRepository implements Repository using PostgreSQL.
type PostgresRepository struct{}

// NewPostgresRepository creates a PostgreSQL transaction repository.
func NewPostgresRepository() *PostgresRepository {
	return &PostgresRepository{}
}

// TryCreate atomically attempts to reserve an idempotency key.
//
// If another transaction is inserting the same key, PostgreSQL waits
// for that transaction to commit or roll back before resolving the
// unique constraint.
//
// A conflict is not treated as an SQL error.
func (r *PostgresRepository) TryCreate(
	ctx context.Context,
	tx pgx.Tx,
	transaction *Transaction,
) (bool, error) {
	const query = `
		INSERT INTO transactions (
			transaction_type,
			idempotency_key,
			request_hash
		)
		VALUES ($1, $2, $3)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id
	`

	err := tx.QueryRow(
		ctx,
		query,
		transaction.Type,
		transaction.IdempotencyKey,
		transaction.RequestHash,
	).Scan(&transaction.ID)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("reserve idempotency key: %w", err)
	}

	return true, nil
}

// Create inserts a new financial transaction.
// The unique constraint on idempotency_key prevents duplicate keys.
func (r *PostgresRepository) Create(
	ctx context.Context,
	tx pgx.Tx,
	transaction *Transaction,
) error {
	const query = `
		INSERT INTO transactions (
			transaction_type,
			idempotency_key,
			request_hash
		)
		VALUES ($1, $2, $3)
		RETURNING id
	`

	err := tx.QueryRow(
		ctx,
		query,
		transaction.Type,
		transaction.IdempotencyKey,
		transaction.RequestHash,
	).Scan(&transaction.ID)

	if err != nil {
		return fmt.Errorf("create transaction: %w", err)
	}

	return nil
}

// GetByIdempotencyKey retrieves an existing financial transaction.
func (r *PostgresRepository) GetByIdempotencyKey(
	ctx context.Context,
	tx pgx.Tx,
	key string,
) (*Transaction, error) {
	const query = `
		SELECT
			id,
			transaction_type,
			idempotency_key,
			request_hash,
			response_status,
			response_body
		FROM transactions
		WHERE idempotency_key = $1
	`

	transaction := &Transaction{}

	err := tx.QueryRow(ctx, query, key).Scan(
		&transaction.ID,
		&transaction.Type,
		&transaction.IdempotencyKey,
		&transaction.RequestHash,
		&transaction.ResponseStatus,
		&transaction.ResponseBody,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"get transaction by idempotency key: %w",
			err,
		)
	}

	return transaction, nil
}

// Complete persists the successful HTTP response.
// The caller must commit the database transaction afterward.
func (r *PostgresRepository) Complete(
	ctx context.Context,
	tx pgx.Tx,
	id int64,
	responseStatus int,
	responseBody []byte,
) error {
	const query = `
		UPDATE transactions
		SET
			response_status = $2,
			response_body = $3
		WHERE id = $1
		  AND response_status IS NULL
	`

	result, err := tx.Exec(
		ctx,
		query,
		id,
		responseStatus,
		responseBody,
	)

	if err != nil {
		return fmt.Errorf("complete transaction: %w", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf(
			"complete transaction: expected one updated row, got %d",
			result.RowsAffected(),
		)
	}

	return nil
}

// Verify at compile time that PostgresRepository implements Repository.
var _ Repository = (*PostgresRepository)(nil)
