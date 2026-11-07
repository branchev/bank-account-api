package account

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/platform/database/testutil"
	"github.com/branchev/bank-account-api/internal/transaction"
)

func TestPostgresService_CreateAccountWithInitialBalance(t *testing.T) {
	pool := testutil.NewPostgresPool(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	service := NewPostgresService(
		pool,
		NewPostgresRepository(),
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	key := fmt.Sprintf("create-account-test-%d", time.Now().UnixNano())

	result, err := service.CreateAccount(ctx, CreateAccountRequest{
		InitialBalanceCents: 10000,
		IdempotencyKey:      key,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	if result.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", result.StatusCode)
	}

	if result.Replay {
		t.Fatal("first request should not be a replay")
	}

	var createdAccount Account
	if err := json.Unmarshal(result.Body, &createdAccount); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if createdAccount.ID <= 0 {
		t.Fatal("expected a valid account ID")
	}

	if createdAccount.Currency != "EUR" {
		t.Fatalf("expected EUR, got %s", createdAccount.Currency)
	}

	if createdAccount.BalanceCents != 10000 {
		t.Fatalf(
			"expected balance 10000, got %d",
			createdAccount.BalanceCents,
		)
	}

	var databaseBalance int64
	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE id = $1`,
		createdAccount.ID,
	).Scan(&databaseBalance)
	if err != nil {
		t.Fatalf("read customer balance: %v", err)
	}

	if databaseBalance != 10000 {
		t.Fatalf("expected database balance 10000, got %d", databaseBalance)
	}

	var transactionID int64
	var storedStatus int

	err = pool.QueryRow(
		ctx,
		`
		SELECT id, response_status
		FROM transactions
		WHERE idempotency_key = $1
		`,
		key,
	).Scan(&transactionID, &storedStatus)
	if err != nil {
		t.Fatalf("read transaction: %v", err)
	}

	if storedStatus != http.StatusCreated {
		t.Fatalf("expected stored status 201, got %d", storedStatus)
	}

	var entryCount int
	var ledgerSum int64

	err = pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*), COALESCE(SUM(amount_cents), 0)
		FROM ledger_entries
		WHERE transaction_id = $1
		`,
		transactionID,
	).Scan(&entryCount, &ledgerSum)
	if err != nil {
		t.Fatalf("read ledger entries: %v", err)
	}

	if entryCount != 2 {
		t.Fatalf("expected 2 ledger entries, got %d", entryCount)
	}

	if ledgerSum != 0 {
		t.Fatalf("expected balanced ledger, got sum %d", ledgerSum)
	}

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
