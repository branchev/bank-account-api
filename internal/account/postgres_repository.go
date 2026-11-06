package account

import (
	"context"
	"fmt"

	"github.com/branchev/bank-account-api/internal/platform/database"
)

type PostgresRepository struct{}

func NewPostgresRepository() *PostgresRepository {
	return &PostgresRepository{}
}

func (r *PostgresRepository) Create(
	ctx context.Context,
	db database.Querier,
	account *Account,
) error {
	const query = `
		INSERT INTO accounts (
			account_type,
			currency,
			balance_cents
		)
		VALUES ('customer', $1, $2)
		RETURNING id
	`

	err := db.QueryRow(
		ctx,
		query,
		account.Currency,
		account.BalanceCents,
	).Scan(&account.ID)

	if err != nil {
		return fmt.Errorf("create account: %w", err)
	}

	return nil
}

func (r *PostgresRepository) GetByID(
	ctx context.Context,
	db database.Querier,
	id int64,
) (*Account, error) {
	const query = `
		SELECT id, currency, balance_cents
		FROM accounts
		WHERE id = $1
		  AND account_type = 'customer'
	`

	account := &Account{}

	err := db.QueryRow(ctx, query, id).Scan(
		&account.ID,
		&account.Currency,
		&account.BalanceCents,
	)
	if err != nil {
		return nil, fmt.Errorf("get account by id %d: %w", id, err)
	}

	return account, nil
}

// Compile-time check that PostgresRepository implements Repository.
var _ Repository = (*PostgresRepository)(nil)
