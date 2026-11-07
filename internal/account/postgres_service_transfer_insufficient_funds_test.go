package account

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/platform/database/testutil"
	"github.com/branchev/bank-account-api/internal/transaction"
)

func TestPostgresService_TransferInsufficientFunds(t *testing.T) {
	pool := testutil.NewPostgresPool(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	service := NewPostgresService(
		pool,
		NewPostgresRepository(),
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	createAccount := func(key string, balance int64) Account {
		t.Helper()

		result, err := service.CreateAccount(ctx, CreateAccountRequest{
			InitialBalanceCents: balance,
			IdempotencyKey:      key,
		})
		if err != nil {
			t.Fatalf("create account: %v", err)
		}

		var account Account
		if err := json.Unmarshal(result.Body, &account); err != nil {
			t.Fatalf("decode account: %v", err)
		}

		return account
	}

	source := createAccount("insufficient-source", 2000)
	destination := createAccount("insufficient-destination", 1000)

	// Attempt to transfer 30 EUR from an account holding only 20 EUR.
	_, err := service.Transfer(ctx, TransferRequest{
		SourceAccountID:      source.ID,
		DestinationAccountID: destination.ID,
		AmountCents:          3000,
		IdempotencyKey:       "insufficient-transfer-001",
	})

	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}

	// Both account balances must remain unchanged.
	var sourceBalance, destinationBalance int64

	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE id = $1`,
		source.ID,
	).Scan(&sourceBalance)
	if err != nil {
		t.Fatalf("read source balance: %v", err)
	}

	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE id = $1`,
		destination.ID,
	).Scan(&destinationBalance)
	if err != nil {
		t.Fatalf("read destination balance: %v", err)
	}

	if sourceBalance != 2000 {
		t.Fatalf("expected source balance 2000, got %d", sourceBalance)
	}

	if destinationBalance != 1000 {
		t.Fatalf("expected destination balance 1000, got %d", destinationBalance)
	}

	// Failed transfers must not leave committed transaction records.
	var transactionCount int

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM transactions WHERE idempotency_key = $1`,
		"insufficient-transfer-001",
	).Scan(&transactionCount)
	if err != nil {
		t.Fatalf("count transactions: %v", err)
	}

	if transactionCount != 0 {
		t.Fatalf("expected 0 committed transactions, got %d", transactionCount)
	}

	// No ledger entries should be created for the failed transfer.
	var ledgerCount int

	err = pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*)
		FROM ledger_entries le
		JOIN transactions t ON t.id = le.transaction_id
		WHERE t.idempotency_key = $1
		`,
		"insufficient-transfer-001",
	).Scan(&ledgerCount)
	if err != nil {
		t.Fatalf("count ledger entries: %v", err)
	}

	if ledgerCount != 0 {
		t.Fatalf("expected 0 ledger entries, got %d", ledgerCount)
	}
}
