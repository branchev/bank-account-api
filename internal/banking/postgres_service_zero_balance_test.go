package banking

import (
	"context"
	"encoding/json"
	accountpkg "github.com/branchev/bank-account-api/internal/account"
	"net/http"
	"testing"
	"time"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/platform/database/testutil"
	"github.com/branchev/bank-account-api/internal/transaction"
)

func TestPostgresService_CreateAccountWithZeroBalance(t *testing.T) {
	pool := testutil.NewPostgresPool(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	service := NewPostgresService(
		pool,
		accountpkg.NewPostgresRepository(),
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	result, err := service.CreateAccount(ctx, CreateAccountRequest{
		InitialBalanceCents: 0,
		IdempotencyKey:      "account-zero-balance-test",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	if result.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", result.StatusCode)
	}

	if result.Replay {
		t.Fatal("first request must not be a replay")
	}

	var createdAccount accountpkg.Account
	if err := json.Unmarshal(result.Body, &createdAccount); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if createdAccount.ID <= 0 {
		t.Fatal("expected a valid account ID")
	}

	if createdAccount.Currency != "EUR" {
		t.Fatalf("expected EUR, got %s", createdAccount.Currency)
	}

	if createdAccount.BalanceCents != 0 {
		t.Fatalf(
			"expected zero balance, got %d",
			createdAccount.BalanceCents,
		)
	}

	// Verify the persisted account.
	var databaseBalance int64
	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE id = $1`,
		createdAccount.ID,
	).Scan(&databaseBalance)
	if err != nil {
		t.Fatalf("read customer balance: %v", err)
	}

	if databaseBalance != 0 {
		t.Fatalf("expected database balance 0, got %d", databaseBalance)
	}

	// Verify the completed financial transaction.
	var transactionID int64
	var storedStatus int

	err = pool.QueryRow(
		ctx,
		`
		SELECT id, response_status
		FROM transactions
		WHERE idempotency_key = $1
		`,
		"account-zero-balance-test",
	).Scan(&transactionID, &storedStatus)
	if err != nil {
		t.Fatalf("read transaction: %v", err)
	}

	if storedStatus != http.StatusCreated {
		t.Fatalf("expected stored status 201, got %d", storedStatus)
	}

	// No money moved, so there must be no ledger entries.
	var ledgerCount int

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM ledger_entries WHERE transaction_id = $1`,
		transactionID,
	).Scan(&ledgerCount)
	if err != nil {
		t.Fatalf("count ledger entries: %v", err)
	}

	if ledgerCount != 0 {
		t.Fatalf("expected no ledger entries, got %d", ledgerCount)
	}

	// The isolated system account starts at zero
	// and must remain unchanged.
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

	if systemBalance != 0 {
		t.Fatalf(
			"expected system balance 0, got %d",
			systemBalance,
		)
	}
}
