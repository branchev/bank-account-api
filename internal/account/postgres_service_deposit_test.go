package account

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/platform/database/testutil"
	"github.com/branchev/bank-account-api/internal/transaction"
)

func TestPostgresService_Deposit(t *testing.T) {
	pool := testutil.NewPostgresPool(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	service := NewPostgresService(
		pool,
		NewPostgresRepository(),
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	// Create a customer account with an initial balance of 100 EUR.
	created, err := service.CreateAccount(ctx, CreateAccountRequest{
		InitialBalanceCents: 10000,
		IdempotencyKey:      "deposit-test-create-account",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	var account Account
	if err := json.Unmarshal(created.Body, &account); err != nil {
		t.Fatalf("decode account: %v", err)
	}

	// Deposit 50 EUR into the existing account.
	result, err := service.Deposit(ctx, DepositRequest{
		AccountID:      account.ID,
		AmountCents:    5000,
		IdempotencyKey: "deposit-test-001",
	})
	if err != nil {
		t.Fatalf("deposit: %v", err)
	}

	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", result.StatusCode)
	}

	if result.Replay {
		t.Fatal("first deposit must not be a replay")
	}

	var updated Account
	if err := json.Unmarshal(result.Body, &updated); err != nil {
		t.Fatalf("decode deposit response: %v", err)
	}

	if updated.ID != account.ID {
		t.Fatalf("expected account ID %d, got %d", account.ID, updated.ID)
	}

	if updated.Currency != "EUR" {
		t.Fatalf("expected EUR, got %s", updated.Currency)
	}

	if updated.BalanceCents != 15000 {
		t.Fatalf("expected balance 15000, got %d", updated.BalanceCents)
	}

	// Verify the persisted customer balance.
	var databaseBalance int64
	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE id = $1`,
		account.ID,
	).Scan(&databaseBalance)
	if err != nil {
		t.Fatalf("read customer balance: %v", err)
	}

	if databaseBalance != 15000 {
		t.Fatalf("expected database balance 15000, got %d", databaseBalance)
	}

	// Verify the deposit transaction and its ledger entries.
	var transactionID int64
	var transactionType string
	var storedStatus int

	err = pool.QueryRow(
		ctx,
		`
		SELECT id, transaction_type, response_status
		FROM transactions
		WHERE idempotency_key = $1
		`,
		"deposit-test-001",
	).Scan(&transactionID, &transactionType, &storedStatus)
	if err != nil {
		t.Fatalf("read deposit transaction: %v", err)
	}

	if transactionType != string(transaction.TypeDeposit) {
		t.Fatalf("expected deposit transaction, got %s", transactionType)
	}

	if storedStatus != http.StatusOK {
		t.Fatalf("expected stored status 200, got %d", storedStatus)
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
		t.Fatalf("read deposit ledger entries: %v", err)
	}

	if entryCount != 2 {
		t.Fatalf("expected 2 ledger entries, got %d", entryCount)
	}

	if ledgerSum != 0 {
		t.Fatalf("expected balanced ledger, got sum %d", ledgerSum)
	}

	// Initial funding was 100 EUR and the deposit adds 50 EUR.
	var systemBalance int64

	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE account_type = 'system'`,
	).Scan(&systemBalance)
	if err != nil {
		t.Fatalf("read system balance: %v", err)
	}

	if systemBalance != -15000 {
		t.Fatalf("expected system balance -15000, got %d", systemBalance)
	}
}
