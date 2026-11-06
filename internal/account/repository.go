package account

import (
	"context"

	"github.com/branchev/bank-account-api/internal/platform/database"
)

type Repository interface {
	Create(
		ctx context.Context,
		db database.Querier,
		account *Account,
	) error

	GetByID(
		ctx context.Context,
		db database.Querier,
		id int64,
	) (*Account, error)
}
