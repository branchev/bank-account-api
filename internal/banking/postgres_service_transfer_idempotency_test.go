package banking

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	accountpkg "github.com/branchev/bank-account-api/internal/account"
	"testing"
	"time"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/platform/database/testutil"
	"github.com/branchev/bank-account-api/internal/transaction"
)

func TestPostgresService_TransferIdempotency(t *testing.T) {
	pool := testutil.NewPostgresPool(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	source := createAccount("transfer-idem-source", 10000)
	destination := createAccount("transfer-idem-destination", 2000)

	request := TransferRequest{
		SourceAccountID:      source.ID,
		DestinationAccountID: destination.ID,
		AmountCents:          3000,
		IdempotencyKey:       "transfer-idem-001",
	}

	first, err := service.Transfer(ctx, request)
	if err != nil {
		t.Fatalf("first transfer: %v", err)
	}

	if first.Replay {
		t.Fatal("first transfer must not be a replay")
	}

	second, err := service.Transfer(ctx, request)
	if err != nil {
		t.Fatalf("repeated transfer: %v", err)
	}

	if !second.Replay {
		t.Fatal("second transfer must be a replay")
	}

	if first.StatusCode != second.StatusCode {
		t.Fatalf("status mismatch: %d vs %d", first.StatusCode, second.StatusCode)
	}

	// PostgreSQL JSONB may normalize JSON formatting.
	// Compare decoded responses rather than raw bytes.
	var firstBody, secondBody any

	if err := json.Unmarshal(first.Body, &firstBody); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if err := json.Unmarshal(second.Body, &secondBody); err != nil {
		t.Fatalf("decode replay response: %v", err)
	}

	firstJSON, _ := json.Marshal(firstBody)
	secondJSON, _ := json.Marshal(secondBody)

	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("replay response differs from original")
	}

	// The transfer must have been executed exactly once.
	var sourceBalance, destinationBalance int64

	if err := pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE id = $1`,
		source.ID,
	).Scan(&sourceBalance); err != nil {
		t.Fatalf("read source balance: %v", err)
	}

	if err := pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE id = $1`,
		destination.ID,
	).Scan(&destinationBalance); err != nil {
		t.Fatalf("read destination balance: %v", err)
	}

	if sourceBalance != 7000 || destinationBalance != 5000 {
		t.Fatalf(
			"unexpected balances after replay: source=%d destination=%d",
			sourceBalance,
			destinationBalance,
		)
	}

	var transactionCount, ledgerCount int

	if err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM transactions WHERE idempotency_key = $1`,
		request.IdempotencyKey,
	).Scan(&transactionCount); err != nil {
		t.Fatalf("count transactions: %v", err)
	}

	if err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*)
		 FROM ledger_entries
		 WHERE transaction_id = (
		     SELECT id FROM transactions WHERE idempotency_key = $1
		 )`,
		request.IdempotencyKey,
	).Scan(&ledgerCount); err != nil {
		t.Fatalf("count ledger entries: %v", err)
	}

	if transactionCount != 1 || ledgerCount != 2 {
		t.Fatalf(
			"expected 1 transaction and 2 ledger entries, got %d and %d",
			transactionCount,
			ledgerCount,
		)
	}

	// Reusing the key with a different amount must fail.
	conflictingRequest := request
	conflictingRequest.AmountCents = 4000

	_, err = service.Transfer(ctx, conflictingRequest)

	if !errors.Is(err, transaction.ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
}
