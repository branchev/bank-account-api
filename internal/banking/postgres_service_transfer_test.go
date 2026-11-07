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

func TestPostgresService_Transfer(t *testing.T) {
	pool := testutil.NewPostgresPool(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	service := NewPostgresService(
		pool,
		accountpkg.NewPostgresRepository(),
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	// Create source account with 100 EUR.
	sourceResult, err := service.CreateAccount(ctx, CreateAccountRequest{
		InitialBalanceCents: 10000,
		IdempotencyKey:      "transfer-test-source",
	})
	if err != nil {
		t.Fatalf("create source account: %v", err)
	}

	var source accountpkg.Account
	if err := json.Unmarshal(sourceResult.Body, &source); err != nil {
		t.Fatalf("decode source account: %v", err)
	}

	// Create destination account with 20 EUR.
	destinationResult, err := service.CreateAccount(ctx, CreateAccountRequest{
		InitialBalanceCents: 2000,
		IdempotencyKey:      "transfer-test-destination",
	})
	if err != nil {
		t.Fatalf("create destination account: %v", err)
	}

	var destination accountpkg.Account
	if err := json.Unmarshal(destinationResult.Body, &destination); err != nil {
		t.Fatalf("decode destination account: %v", err)
	}

	// Transfer 30 EUR from source to destination.
	result, err := service.Transfer(ctx, TransferRequest{
		SourceAccountID:      source.ID,
		DestinationAccountID: destination.ID,
		AmountCents:          3000,
		IdempotencyKey:       "transfer-test-001",
	})
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}

	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", result.StatusCode)
	}

	if result.Replay {
		t.Fatal("first transfer must not be a replay")
	}

	// Verify persisted balances.
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

	if sourceBalance != 7000 {
		t.Fatalf("expected source balance 7000, got %d", sourceBalance)
	}

	if destinationBalance != 5000 {
		t.Fatalf("expected destination balance 5000, got %d", destinationBalance)
	}

	// Verify the transaction record.
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
		"transfer-test-001",
	).Scan(&transactionID, &transactionType, &storedStatus)
	if err != nil {
		t.Fatalf("read transfer transaction: %v", err)
	}

	if transactionType != string(transaction.TypeTransfer) {
		t.Fatalf("expected transfer transaction, got %s", transactionType)
	}

	if storedStatus != http.StatusOK {
		t.Fatalf("expected stored status 200, got %d", storedStatus)
	}

	// Verify both ledger entries individually.
	var sourceEntry, destinationEntry int64

	err = pool.QueryRow(
		ctx,
		`
		SELECT amount_cents
		FROM ledger_entries
		WHERE transaction_id = $1 AND account_id = $2
		`,
		transactionID,
		source.ID,
	).Scan(&sourceEntry)
	if err != nil {
		t.Fatalf("read source ledger entry: %v", err)
	}

	err = pool.QueryRow(
		ctx,
		`
		SELECT amount_cents
		FROM ledger_entries
		WHERE transaction_id = $1 AND account_id = $2
		`,
		transactionID,
		destination.ID,
	).Scan(&destinationEntry)
	if err != nil {
		t.Fatalf("read destination ledger entry: %v", err)
	}

	if sourceEntry != -3000 {
		t.Fatalf("expected source ledger entry -3000, got %d", sourceEntry)
	}

	if destinationEntry != 3000 {
		t.Fatalf("expected destination ledger entry 3000, got %d", destinationEntry)
	}

	// Verify exactly two entries and a balanced ledger.
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
		t.Fatalf("read transfer ledger summary: %v", err)
	}

	if entryCount != 2 {
		t.Fatalf("expected 2 ledger entries, got %d", entryCount)
	}

	if ledgerSum != 0 {
		t.Fatalf("expected balanced ledger, got sum %d", ledgerSum)
	}

	// Internal system account must remain unchanged by the transfer.
	var systemBalance int64

	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE account_type = 'system'`,
	).Scan(&systemBalance)
	if err != nil {
		t.Fatalf("read system balance: %v", err)
	}

	if systemBalance != -12000 {
		t.Fatalf("expected system balance -12000, got %d", systemBalance)
	}
}
