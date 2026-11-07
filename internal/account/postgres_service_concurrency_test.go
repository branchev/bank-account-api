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

func TestPostgresService_CreateAccountConcurrentIdempotency(t *testing.T) {
	pool := testutil.NewPostgresPool(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	service := NewPostgresService(
		pool,
		NewPostgresRepository(),
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	request := CreateAccountRequest{
		InitialBalanceCents: 10000,
		IdempotencyKey:      "concurrent-account-creation",
	}

	const workers = 2

	type outcome struct {
		result CreateAccountResult
		err    error
	}

	results := make([]outcome, workers)

	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func(index int) {
			defer wg.Done()

			// Both goroutines wait for the same start signal.
			<-start

			results[index].result, results[index].err =
				service.CreateAccount(ctx, request)
		}(i)
	}

	// Release both goroutines simultaneously.
	close(start)
	wg.Wait()

	var createdCount int
	var replayCount int
	var accountID int64

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
			replayCount++
		} else {
			createdCount++
		}

		var account Account
		if err := json.Unmarshal(outcome.result.Body, &account); err != nil {
			t.Fatalf("request %d: decode response: %v", i, err)
		}

		if account.ID <= 0 {
			t.Fatalf("request %d: invalid account ID", i)
		}

		if account.BalanceCents != 10000 {
			t.Fatalf(
				"request %d: expected balance 10000, got %d",
				i,
				account.BalanceCents,
			)
		}

		if accountID == 0 {
			accountID = account.ID
		} else if account.ID != accountID {
			t.Fatalf(
				"different accounts created: %d and %d",
				accountID,
				account.ID,
			)
		}
	}

	if createdCount != 1 {
		t.Fatalf("expected 1 creation, got %d", createdCount)
	}

	if replayCount != 1 {
		t.Fatalf("expected 1 replay, got %d", replayCount)
	}

	// Verify the database contains exactly one customer account.
	var accountCount int
	err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM accounts WHERE account_type = 'customer'`,
	).Scan(&accountCount)
	if err != nil {
		t.Fatalf("count accounts: %v", err)
	}

	if accountCount != 1 {
		t.Fatalf("expected 1 customer account, got %d", accountCount)
	}

	// Exactly one financial transaction should be committed.
	var transactionCount int
	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM transactions`,
	).Scan(&transactionCount)
	if err != nil {
		t.Fatalf("count transactions: %v", err)
	}

	if transactionCount != 1 {
		t.Fatalf("expected 1 transaction, got %d", transactionCount)
	}

	// Initial funding must create exactly two balanced ledger entries.
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

	if ledgerCount != 2 {
		t.Fatalf("expected 2 ledger entries, got %d", ledgerCount)
	}

	if ledgerSum != 0 {
		t.Fatalf("expected balanced ledger, got sum %d", ledgerSum)
	}

	// The system account must be debited only once.
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

	if systemBalance != -10000 {
		t.Fatalf(
			"expected system balance -10000, got %d",
			systemBalance,
		)
	}
}
