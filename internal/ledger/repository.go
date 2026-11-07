package ledger

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Repository defines the persistence operations for ledger entries.
type Repository interface {
	CreateEntries(
		ctx context.Context,
		tx pgx.Tx,
		entries []Entry,
	) error
}
