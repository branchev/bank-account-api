package ledger

import (
	"context"
	"errors"
	"math"
	"os"
	"testing"

	"github.com/branchev/bank-account-api/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func setupTestTransaction(t *testing.T) (context.Context, pgx.Tx) {
	t.Helper()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	ctx := context.Background()

	pool, err := database.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}

	t.Cleanup(pool.Close)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}

	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rollback test transaction: %v", err)
		}
	})

	return ctx, tx
}

func TestPostgresRepository_CreateEntries(t *testing.T) {
	ctx, tx := setupTestTransaction(t)

	// Find the system funding account without hardcoding its ID.
	var systemAccountID int64

	err := tx.QueryRow(ctx, `
		SELECT id
		FROM accounts
		WHERE account_type = 'system'
	`).Scan(&systemAccountID)
	if err != nil {
		t.Fatalf("find system account: %v", err)
	}

	// Create a customer account inside the test transaction.
	var customerAccountID int64

	err = tx.QueryRow(ctx, `
		INSERT INTO accounts (account_type, currency, balance_cents)
		VALUES ('customer', 'EUR', 0)
		RETURNING id
	`).Scan(&customerAccountID)
	if err != nil {
		t.Fatalf("create customer account: %v", err)
	}

	// Create transaction metadata required by the ledger foreign key.
	var transactionID int64

	err = tx.QueryRow(ctx, `
		INSERT INTO transactions (
			transaction_type,
			idempotency_key,
			request_hash
		)
		VALUES ('deposit', $1, 'test-request-hash')
		RETURNING id
	`, t.Name()).Scan(&transactionID)
	if err != nil {
		t.Fatalf("create transaction metadata: %v", err)
	}

	repo := NewPostgresRepository()

	entries := []Entry{
		{
			TransactionID: transactionID,
			AccountID:     systemAccountID,
			AmountCents:   -10000,
		},
		{
			TransactionID: transactionID,
			AccountID:     customerAccountID,
			AmountCents:   10000,
		},
	}

	if err := repo.CreateEntries(ctx, tx, entries); err != nil {
		t.Fatalf("create ledger entries: %v", err)
	}

	var count int
	var total int64

	err = tx.QueryRow(ctx, `
		SELECT COUNT(*), SUM(amount_cents)
		FROM ledger_entries
		WHERE transaction_id = $1
	`, transactionID).Scan(&count, &total)
	if err != nil {
		t.Fatalf("query ledger entries: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 ledger entries, got %d", count)
	}

	if total != 0 {
		t.Errorf("expected balanced ledger total 0, got %d", total)
	}
}

func TestPostgresRepository_CreateEntries_Unbalanced(t *testing.T) {
	ctx, tx := setupTestTransaction(t)

	var systemAccountID int64

	err := tx.QueryRow(ctx, `
		SELECT id
		FROM accounts
		WHERE account_type = 'system'
	`).Scan(&systemAccountID)
	if err != nil {
		t.Fatalf("find system account: %v", err)
	}

	var customerAccountID int64

	err = tx.QueryRow(ctx, `
		INSERT INTO accounts (account_type, currency, balance_cents)
		VALUES ('customer', 'EUR', 0)
		RETURNING id
	`).Scan(&customerAccountID)
	if err != nil {
		t.Fatalf("create customer account: %v", err)
	}

	var transactionID int64

	err = tx.QueryRow(ctx, `
		INSERT INTO transactions (
			transaction_type,
			idempotency_key,
			request_hash
		)
		VALUES ('deposit', $1, 'test-request-hash')
		RETURNING id
	`, t.Name()).Scan(&transactionID)
	if err != nil {
		t.Fatalf("create transaction metadata: %v", err)
	}

	repo := NewPostgresRepository()

	entries := []Entry{
		{
			TransactionID: transactionID,
			AccountID:     systemAccountID,
			AmountCents:   -10000,
		},
		{
			TransactionID: transactionID,
			AccountID:     customerAccountID,
			AmountCents:   9000,
		},
	}

	err = repo.CreateEntries(ctx, tx, entries)
	if err == nil {
		t.Fatal("expected error for unbalanced ledger entries")
	}

	var count int

	err = tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM ledger_entries
		WHERE transaction_id = $1
	`, transactionID).Scan(&count)
	if err != nil {
		t.Fatalf("count ledger entries: %v", err)
	}

	if count != 0 {
		t.Errorf("expected no ledger entries, got %d", count)
	}
}

func TestPostgresRepository_CreateEntries_Overflow(t *testing.T) {
	ctx, tx := setupTestTransaction(t)

	repo := NewPostgresRepository()

	entries := []Entry{
		{
			TransactionID: 1,
			AccountID:     1,
			AmountCents:   math.MaxInt64,
		},
		{
			TransactionID: 1,
			AccountID:     2,
			AmountCents:   1,
		},
		{
			TransactionID: 1,
			AccountID:     3,
			AmountCents:   math.MinInt64,
		},
	}

	err := repo.CreateEntries(ctx, tx, entries)

	if err == nil {
		t.Fatal("expected integer overflow error")
	}

	if err.Error() != "ledger amount sum overflow" {
		t.Errorf("expected overflow error, got %v", err)
	}
}

func TestPostgresRepository_CreateEntries_Validation(t *testing.T) {
	tests := []struct {
		name        string
		entries     []Entry
		expectedErr string
	}{
		{
			name:        "empty entries",
			entries:     nil,
			expectedErr: "ledger posting requires at least two entries",
		},
		{
			name: "single entry",
			entries: []Entry{
				{TransactionID: 1, AccountID: 1, AmountCents: 100},
			},
			expectedErr: "ledger posting requires at least two entries",
		},
		{
			name: "invalid transaction ID",
			entries: []Entry{
				{TransactionID: 0, AccountID: 1, AmountCents: -100},
				{TransactionID: 0, AccountID: 2, AmountCents: 100},
			},
			expectedErr: "invalid ledger transaction ID",
		},
		{
			name: "different transaction IDs",
			entries: []Entry{
				{TransactionID: 1, AccountID: 1, AmountCents: -100},
				{TransactionID: 2, AccountID: 2, AmountCents: 100},
			},
			expectedErr: "ledger entries must belong to the same transaction",
		},
		{
			name: "invalid account ID",
			entries: []Entry{
				{TransactionID: 1, AccountID: 0, AmountCents: -100},
				{TransactionID: 1, AccountID: 2, AmountCents: 100},
			},
			expectedErr: "invalid ledger account ID",
		},
		{
			name: "zero amount",
			entries: []Entry{
				{TransactionID: 1, AccountID: 1, AmountCents: 0},
				{TransactionID: 1, AccountID: 2, AmountCents: 100},
			},
			expectedErr: "ledger entry amount cannot be zero",
		},
		{
			name: "unbalanced entries",
			entries: []Entry{
				{TransactionID: 1, AccountID: 1, AmountCents: -100},
				{TransactionID: 1, AccountID: 2, AmountCents: 90},
			},
			expectedErr: "unbalanced ledger posting: sum is -10 cents",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, tx := setupTestTransaction(t)
			repo := NewPostgresRepository()

			err := repo.CreateEntries(ctx, tx, tt.entries)

			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.expectedErr)
			}

			if err.Error() != tt.expectedErr {
				t.Errorf("expected error %q, got %q", tt.expectedErr, err.Error())
			}
		})
	}
}
