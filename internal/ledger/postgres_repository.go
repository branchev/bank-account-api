package ledger

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"
)

// PostgresRepository implements Repository using PostgreSQL.
type PostgresRepository struct{}

// NewPostgresRepository creates a PostgreSQL ledger repository.
func NewPostgresRepository() *PostgresRepository {
	return &PostgresRepository{}
}

// CreateEntries validates and inserts balanced ledger entries.
// The caller owns the PostgreSQL transaction and must commit or roll it back.
func (r *PostgresRepository) CreateEntries(
	ctx context.Context,
	tx pgx.Tx,
	entries []Entry,
) error {
	if len(entries) < 2 {
		return errors.New("ledger posting requires at least two entries")
	}

	transactionID := entries[0].TransactionID
	if transactionID <= 0 {
		return errors.New("invalid ledger transaction ID")
	}

	var sum int64

	for _, entry := range entries {
		if entry.TransactionID != transactionID {
			return errors.New("ledger entries must belong to the same transaction")
		}

		if entry.AccountID <= 0 {
			return errors.New("invalid ledger account ID")
		}

		if entry.AmountCents == 0 {
			return errors.New("ledger entry amount cannot be zero")
		}

		// Detect integer overflow before adding signed amounts.
		if (entry.AmountCents > 0 && sum > math.MaxInt64-entry.AmountCents) ||
			(entry.AmountCents < 0 && sum < math.MinInt64-entry.AmountCents) {
			return errors.New("ledger amount sum overflow")
		}

		sum += entry.AmountCents
	}

	if sum != 0 {
		return fmt.Errorf(
			"unbalanced ledger posting: sum is %d cents",
			sum,
		)
	}

	const query = `
		INSERT INTO ledger_entries (
			transaction_id,
			account_id,
			amount_cents
		)
		VALUES ($1, $2, $3)
	`

	for _, entry := range entries {
		_, err := tx.Exec(
			ctx,
			query,
			entry.TransactionID,
			entry.AccountID,
			entry.AmountCents,
		)
		if err != nil {
			return fmt.Errorf("create ledger entry: %w", err)
		}
	}

	return nil
}

// Verify at compile time that PostgresRepository implements Repository.
var _ Repository = (*PostgresRepository)(nil)
