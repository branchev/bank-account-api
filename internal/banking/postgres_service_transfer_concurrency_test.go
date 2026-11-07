package banking

import (
	"context"
	"encoding/json"
	"errors"
	accountpkg "github.com/branchev/bank-account-api/internal/account"
	"sync"
	"testing"
	"time"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/platform/database/testutil"
	"github.com/branchev/bank-account-api/internal/transaction"
)

func TestPostgresService_TransferConcurrentSpending(t *testing.T) {
	pool := testutil.NewPostgresPool(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	service := NewPostgresService(
		pool,
		accountpkg.NewPostgresRepository(),
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	createAccount := func(key string, balance int64) accountpkg.Account {
		t.Helper()

		result, err := service.CreateAccount(ctx, CreateAccountRequest{
			InitialBalanceCents: balance,
			IdempotencyKey:      key,
		})
		if err != nil {
			t.Fatalf("create account: %v", err)
		}

		var account accountpkg.Account
		if err := json.Unmarshal(result.Body, &account); err != nil {
			t.Fatalf("decode account: %v", err)
		}

		return account
	}

	source := createAccount("concurrent-source", 10000)
	destinationB := createAccount("concurrent-destination-b", 0)
	destinationC := createAccount("concurrent-destination-c", 0)

	requests := []TransferRequest{
		{
			SourceAccountID:      source.ID,
			DestinationAccountID: destinationB.ID,
			AmountCents:          8000,
			IdempotencyKey:       "concurrent-transfer-b",
		},
		{
			SourceAccountID:      source.ID,
			DestinationAccountID: destinationC.ID,
			AmountCents:          8000,
			IdempotencyKey:       "concurrent-transfer-c",
		},
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, len(requests))

	for _, request := range requests {
		wg.Add(1)

		go func(req TransferRequest) {
			defer wg.Done()

			// Wait until both goroutines are ready to start.
			<-start

			_, err := service.Transfer(ctx, req)
			results <- err
		}(request)
	}

	close(start)

	wg.Wait()
	close(results)

	successCount := 0
	insufficientCount := 0

	for err := range results {
		switch {
		case err == nil:
			successCount++
		case errors.Is(err, ErrInsufficientFunds):
			insufficientCount++
		default:
			t.Fatalf("unexpected transfer error: %v", err)
		}
	}

	if successCount != 1 {
		t.Fatalf("expected 1 successful transfer, got %d", successCount)
	}

	if insufficientCount != 1 {
		t.Fatalf("expected 1 insufficient funds error, got %d", insufficientCount)
	}

	readBalance := func(accountID int64) int64 {
		t.Helper()

		var balance int64

		err := pool.QueryRow(
			ctx,
			`SELECT balance_cents FROM accounts WHERE id = $1`,
			accountID,
		).Scan(&balance)
		if err != nil {
			t.Fatalf("read account balance: %v", err)
		}

		return balance
	}

	sourceBalance := readBalance(source.ID)
	balanceB := readBalance(destinationB.ID)
	balanceC := readBalance(destinationC.ID)

	if sourceBalance != 2000 {
		t.Fatalf("expected source balance 2000, got %d", sourceBalance)
	}

	if balanceB+balanceC != 8000 {
		t.Fatalf(
			"expected total destination balance 8000, got %d",
			balanceB+balanceC,
		)
	}

	if balanceB+balanceC+sourceBalance != 10000 {
		t.Fatal("total customer funds changed")
	}

	// Exactly one transfer transaction must be committed.
	var transactionCount int

	err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM transactions WHERE transaction_type = 'transfer'`,
	).Scan(&transactionCount)
	if err != nil {
		t.Fatalf("count transfer transactions: %v", err)
	}

	if transactionCount != 1 {
		t.Fatalf("expected 1 committed transfer, got %d", transactionCount)
	}

	// The successful transfer must have exactly two balanced ledger entries.
	var entryCount int
	var ledgerSum int64

	err = pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*), COALESCE(SUM(le.amount_cents), 0)
		FROM ledger_entries le
		JOIN transactions t ON t.id = le.transaction_id
		WHERE t.transaction_type = 'transfer'
		`,
	).Scan(&entryCount, &ledgerSum)
	if err != nil {
		t.Fatalf("read transfer ledger: %v", err)
	}

	if entryCount != 2 || ledgerSum != 0 {
		t.Fatalf(
			"expected 2 balanced ledger entries, got count=%d sum=%d",
			entryCount,
			ledgerSum,
		)
	}
}
