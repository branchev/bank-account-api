package ledger

import (
	"context"

	"github.com/branchev/bank-account-api/internal/platform/database"
)

type Repository interface {
	CreateEntries(
		ctx context.Context,
		db database.Querier,
		entries []Entry,
	) error
}
