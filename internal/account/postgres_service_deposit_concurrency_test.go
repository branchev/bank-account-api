package account

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/platform/database/testutil"
	"github.com/branchev/bank-account-api/internal/transaction"
)

func TestPostgresService_ConcurrentDeposits(t *testing.T) {
	pool := testutil.NewPostgresPool(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	service := NewPostgresService(
		pool,
		NewPostgresRepository(),
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	created, err := service.CreateAccount(ctx, CreateAccountRequest{
		InitialBalanceCents: 10000,
		IdempotencyKey:      "concurrent-deposits-create-account",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	var account Account
	if err := json.Unmarshal(created.Body, &account); err != nil {
		t.Fatalf("decode account: %v", err)
	}

	requests := []DepositRequest{
		{
			AccountID:      account.ID,
			AmountCents:    5000,
			IdempotencyKey: "concurrent-deposit-5000",
		},
		{
			AccountID:      account.ID,
			AmountCents:    7000,
			IdempotencyKey: "concurrent-deposit-7000",
		},
	}

	type outcome struct {
		result DepositResult
		err    error
	}

	results := make([]outcome, len(requests))
	start := make(chan struct{})

	var wg sync.WaitGroup

	for i, request := range requests {
		wg.Add(1)

		go func(index int, req DepositRequest) {
			defer wg.Done()

			<-start

			results[index].result, results[index].err =
				service.Deposit(ctx, req)
		}(i, request)
	}

	close(start)
	wg.Wait()

	for i, outcome := range results {
		if outcome.err != nil {
			t.Fatalf("deposit %d failed: %v", i, outcome.err)
		}

		if outcome.result.StatusCode != http.StatusOK {
			t.Fatalf(
				"deposit %d: expected 200, got %d",
				i,
				outcome.result.StatusCode,
			)
		}

		if outcome.result.Replay {
			t.Fatalf("deposit %d: unexpected replay", i)
		}
	}

	// Both deposits must be reflected in the persisted balance.
	var balance int64

	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE id = $1`,
		account.ID,
	).Scan(&balance)
	if err != nil {
		t.Fatalf("read account balance: %v", err)
	}

	if balance != 22000 {
		t.Fatalf("expected balance 22000, got %d", balance)
	}

	// System account must reflect initial funding and both deposits.
	var systemBalance int64

	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE account_type = 'system'`,
	).Scan(&systemBalance)
	if err != nil {
		t.Fatalf("read system balance: %v", err)
	}

	if systemBalance != -22000 {
		t.Fatalf("expected system balance -22000, got %d", systemBalance)
	}

	// Exactly two deposit transactions should exist.
	var depositCount int

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM transactions WHERE transaction_type = 'deposit'`,
	).Scan(&depositCount)
	if err != nil {
		t.Fatalf("count deposit transactions: %v", err)
	}

	if depositCount != 2 {
		t.Fatalf("expected 2 deposit transactions, got %d", depositCount)
	}

	// Each deposit creates two ledger entries.
	var ledgerCount int
	var ledgerSum int64

	err = pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*), COALESCE(SUM(le.amount_cents), 0)
		FROM ledger_entries le
		JOIN transactions t ON t.id = le.transaction_id
		WHERE t.transaction_type = 'deposit'
		`,
	).Scan(&ledgerCount, &ledgerSum)
	if err != nil {
		t.Fatalf("read deposit ledger: %v", err)
	}

	if ledgerCount != 4 {
		t.Fatalf("expected 4 deposit ledger entries, got %d", ledgerCount)
	}

	if ledgerSum != 0 {
		t.Fatalf("expected balanced deposit ledger, got %d", ledgerSum)
	}
}
