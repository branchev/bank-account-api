package account

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/branchev/bank-account-api/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func TestPostgresRepository_Create(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	ctx := context.Background()

	pool, err := database.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	repo := NewPostgresRepository()

	account := &Account{
		Currency:     "EUR",
		BalanceCents: 0,
	}

	err = repo.Create(ctx, tx, account)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	if account.ID <= 0 {
		t.Fatalf("expected positive account ID, got %d", account.ID)
	}

	var accountType string
	var currency string
	var balanceCents int64

	err = tx.QueryRow(
		ctx,
		`
		SELECT account_type, currency, balance_cents
		FROM accounts
		WHERE id = $1
		`,
		account.ID,
	).Scan(&accountType, &currency, &balanceCents)

	if err != nil {
		t.Fatalf("query created account: %v", err)
	}

	if accountType != "customer" {
		t.Errorf("expected customer account, got %q", accountType)
	}

	if currency != "EUR" {
		t.Errorf("expected EUR currency, got %q", currency)
	}

	if balanceCents != 0 {
		t.Errorf("expected zero balance, got %d", balanceCents)
	}
}

func TestPostgresRepository_GetByID(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	ctx := context.Background()

	pool, err := database.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	repo := NewPostgresRepository()

	created := &Account{
		Currency:     "EUR",
		BalanceCents: 0,
	}

	if err := repo.Create(ctx, tx, created); err != nil {
		t.Fatalf("create account: %v", err)
	}

	found, err := repo.GetByID(ctx, tx, created.ID)
	if err != nil {
		t.Fatalf("get account by ID: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("expected ID %d, got %d", created.ID, found.ID)
	}

	if found.Currency != "EUR" {
		t.Errorf("expected EUR currency, got %q", found.Currency)
	}

	if found.BalanceCents != 0 {
		t.Errorf("expected zero balance, got %d", found.BalanceCents)
	}
}

func TestPostgresRepository_GetByID_NotFound(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	ctx := context.Background()

	pool, err := database.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	repo := NewPostgresRepository()

	// PostgreSQL identity IDs are positive.
	// No account should have this ID.
	const nonexistentID int64 = -1

	found, err := repo.GetByID(ctx, tx, nonexistentID)

	if found != nil {
		t.Errorf("expected nil account, got %+v", found)
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("expected pgx.ErrNoRows, got %v", err)
	}
}

func TestPostgresRepository_GetByID_SystemAccount(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	ctx := context.Background()

	pool, err := database.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	var systemAccountID int64

	err = tx.QueryRow(
		ctx,
		`
		SELECT id
		FROM accounts
		WHERE account_type = 'system'
		`,
	).Scan(&systemAccountID)

	if err != nil {
		t.Fatalf("get system account ID: %v", err)
	}

	repo := NewPostgresRepository()

	found, err := repo.GetByID(ctx, tx, systemAccountID)

	if found != nil {
		t.Errorf("expected nil account, got %+v", found)
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("expected pgx.ErrNoRows, got %v", err)
	}
}

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

func TestPostgresRepository_Create_Rollback(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	ctx := context.Background()

	pool, err := database.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	repo := NewPostgresRepository()

	account := &Account{
		Currency:     "EUR",
		BalanceCents: 0,
	}

	if err := repo.Create(ctx, tx, account); err != nil {
		t.Fatalf("create account: %v", err)
	}

	// The account exists inside the uncommitted transaction.
	created, err := repo.GetByID(ctx, tx, account.ID)
	if err != nil {
		t.Fatalf("get account before rollback: %v", err)
	}

	if created.ID != account.ID {
		t.Fatalf("expected account ID %d, got %d", account.ID, created.ID)
	}

	// Cancel all changes made by the transaction.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback transaction: %v", err)
	}

	// Query through the pool, outside the rolled-back transaction.
	found, err := repo.GetByID(ctx, pool, account.ID)

	if found != nil {
		t.Errorf("expected nil account after rollback, got %+v", found)
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("expected pgx.ErrNoRows after rollback, got %v", err)
	}
}

func TestPostgresRepository_Create_AlwaysStartsWithZeroBalance(t *testing.T) {
	ctx, tx := setupTestTransaction(t)
	repo := NewPostgresRepository()

	account := &Account{
		Currency:     "EUR",
		BalanceCents: 10000,
	}

	if err := repo.Create(ctx, tx, account); err != nil {
		t.Fatalf("create account: %v", err)
	}

	if account.BalanceCents != 0 {
		t.Errorf("expected in-memory balance 0, got %d", account.BalanceCents)
	}

	created, err := repo.GetByID(ctx, tx, account.ID)
	if err != nil {
		t.Fatalf("get created account: %v", err)
	}

	if created.BalanceCents != 0 {
		t.Errorf("expected persisted balance 0, got %d", created.BalanceCents)
	}
}
