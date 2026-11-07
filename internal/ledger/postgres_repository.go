package ledger

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/branchev/bank-account-api/internal/platform/database"
)

type PostgresRepository struct{}

func NewPostgresRepository() *PostgresRepository {
	return &PostgresRepository{}
}

func (r *PostgresRepository) CreateEntries(
	ctx context.Context,
	db database.Querier,
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

		// Check for integer overflow before adding signed amounts.
		if (entry.AmountCents > 0 && sum > math.MaxInt64-entry.AmountCents) ||
			(entry.AmountCents < 0 && sum < math.MinInt64-entry.AmountCents) {
			return errors.New("ledger amount sum overflow")
		}

		sum += entry.AmountCents
	}

	if sum != 0 {
		return fmt.Errorf("unbalanced ledger posting: sum is %d cents", sum)
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
		_, err := db.Exec(
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

var _ Repository = (*PostgresRepository)(nil)
