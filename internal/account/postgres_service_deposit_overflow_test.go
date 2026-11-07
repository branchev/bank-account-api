package account

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/platform/database/testutil"
	"github.com/branchev/bank-account-api/internal/transaction"
)

func TestPostgresService_DepositOverflow(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	ctx := context.Background()

	service := NewPostgresService(
		pool,
		NewPostgresRepository(),
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	// Create a customer account with zero initial balance.
	created, err := service.CreateAccount(ctx, CreateAccountRequest{
		IdempotencyKey: "deposit-overflow-create",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	var account Account
	if err := json.Unmarshal(created.Body, &account); err != nil {
		t.Fatalf("decode account: %v", err)
	}

	// Set up the boundary condition directly in the isolated test database.
	_, err = pool.Exec(
		ctx,
		`UPDATE accounts SET balance_cents = $1 WHERE id = $2`,
		math.MaxInt64,
		account.ID,
	)
	if err != nil {
		t.Fatalf("set maximum account balance: %v", err)
	}

	// Attempt a deposit that would exceed int64's maximum value.
	_, err = service.Deposit(ctx, DepositRequest{
		AccountID:      account.ID,
		AmountCents:    1,
		IdempotencyKey: "deposit-overflow-attempt",
	})

	if !errors.Is(err, ErrBalanceOverflow) {
		t.Fatalf("expected ErrBalanceOverflow, got %v", err)
	}

	// Customer balance must remain unchanged.
	var customerBalance int64
	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE id = $1`,
		account.ID,
	).Scan(&customerBalance)
	if err != nil {
		t.Fatalf("read customer balance: %v", err)
	}

	if customerBalance != math.MaxInt64 {
		t.Fatalf(
			"expected balance %d, got %d",
			int64(math.MaxInt64),
			customerBalance,
		)
	}

	// System balance must remain unchanged.
	var systemBalance int64
	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE account_type = 'system'`,
	).Scan(&systemBalance)
	if err != nil {
		t.Fatalf("read system balance: %v", err)
	}

	if systemBalance != 0 {
		t.Fatalf("expected system balance 0, got %d", systemBalance)
	}

	// A rejected deposit must not leave committed transaction records.
	var depositCount int
	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM transactions WHERE transaction_type = 'deposit'`,
	).Scan(&depositCount)
	if err != nil {
		t.Fatalf("count deposit transactions: %v", err)
	}

	if depositCount != 0 {
		t.Fatalf("expected 0 deposit transactions, got %d", depositCount)
	}

	// No ledger entries should exist for the rejected deposit.
	var depositLedgerCount int
	err = pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*)
		FROM ledger_entries le
		JOIN transactions t ON t.id = le.transaction_id
		WHERE t.transaction_type = 'deposit'
		`,
	).Scan(&depositLedgerCount)
	if err != nil {
		t.Fatalf("count deposit ledger entries: %v", err)
	}

	if depositLedgerCount != 0 {
		t.Fatalf(
			"expected 0 deposit ledger entries, got %d",
			depositLedgerCount,
		)
	}
}
