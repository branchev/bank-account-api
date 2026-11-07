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

func TestPostgresService_ConcurrentInitialFunding(t *testing.T) {
	pool := testutil.NewPostgresPool(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)
	defer cancel()

	service := NewPostgresService(
		pool,
		NewPostgresRepository(),
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	requests := []CreateAccountRequest{
		{
			InitialBalanceCents: 10000,
			IdempotencyKey:      "concurrent-funding-100",
		},
		{
			InitialBalanceCents: 20000,
			IdempotencyKey:      "concurrent-funding-200",
		},
	}

	type outcome struct {
		result CreateAccountResult
		err    error
	}

	results := make([]outcome, len(requests))

	start := make(chan struct{})
	var wg sync.WaitGroup

	for i, request := range requests {
		wg.Add(1)

		go func(index int, req CreateAccountRequest) {
			defer wg.Done()

			<-start

			results[index].result, results[index].err =
				service.CreateAccount(ctx, req)
		}(i, request)
	}

	close(start)
	wg.Wait()

	// Both independent requests must succeed.
	for i, outcome := range results {
		if outcome.err != nil {
			t.Fatalf("request %d failed: %v", i, outcome.err)
		}

		if outcome.result.StatusCode != http.StatusCreated {
			t.Fatalf(
				"request %d: expected 201, got %d",
				i,
				outcome.result.StatusCode,
			)
		}

		if outcome.result.Replay {
			t.Fatalf("request %d: unexpected replay", i)
		}

		var createdAccount Account

		if err := json.Unmarshal(
			outcome.result.Body,
			&createdAccount,
		); err != nil {
			t.Fatalf("request %d: decode response: %v", i, err)
		}

		if createdAccount.BalanceCents != requests[i].InitialBalanceCents {
			t.Fatalf(
				"request %d: expected balance %d, got %d",
				i,
				requests[i].InitialBalanceCents,
				createdAccount.BalanceCents,
			)
		}
	}

	// Exactly two customer accounts must exist.
	var accountCount int

	err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM accounts WHERE account_type = 'customer'`,
	).Scan(&accountCount)

	if err != nil {
		t.Fatalf("count accounts: %v", err)
	}

	if accountCount != 2 {
		t.Fatalf("expected 2 customer accounts, got %d", accountCount)
	}

	// Each request must have its own financial transaction.
	var transactionCount int

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM transactions`,
	).Scan(&transactionCount)

	if err != nil {
		t.Fatalf("count transactions: %v", err)
	}

	if transactionCount != 2 {
		t.Fatalf("expected 2 transactions, got %d", transactionCount)
	}

	// Each funding transaction produces two ledger entries.
	var ledgerCount int
	var ledgerSum int64

	err = pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*), COALESCE(SUM(amount_cents), 0)
		FROM ledger_entries
		`,
	).Scan(&ledgerCount, &ledgerSum)

	if err != nil {
		t.Fatalf("read ledger entries: %v", err)
	}

	if ledgerCount != 4 {
		t.Fatalf("expected 4 ledger entries, got %d", ledgerCount)
	}

	if ledgerSum != 0 {
		t.Fatalf("expected balanced ledger, got sum %d", ledgerSum)
	}

	// Both debits must be reflected in the system account.
	var systemBalance int64

	err = pool.QueryRow(
		ctx,
		`
		SELECT balance_cents
		FROM accounts
		WHERE account_type = 'system'
		`,
	).Scan(&systemBalance)

	if err != nil {
		t.Fatalf("read system balance: %v", err)
	}

	if systemBalance != -30000 {
		t.Fatalf(
			"expected system balance -30000, got %d",
			systemBalance,
		)
	}
}
