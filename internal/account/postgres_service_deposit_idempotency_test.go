package account

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/platform/database/testutil"
	"github.com/branchev/bank-account-api/internal/transaction"
)

func TestPostgresService_DepositIdempotency(t *testing.T) {
	pool := testutil.NewPostgresPool(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	service := NewPostgresService(
		pool,
		NewPostgresRepository(),
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	createAccount := func(key string) int64 {
		t.Helper()

		result, err := service.CreateAccount(ctx, CreateAccountRequest{
			IdempotencyKey: key,
		})
		if err != nil {
			t.Fatalf("create account: %v", err)
		}

		var account Account
		if err := json.Unmarshal(result.Body, &account); err != nil {
			t.Fatalf("decode account: %v", err)
		}

		return account.ID
	}

	accountID := createAccount("deposit-idempotency-account-1")
	otherAccountID := createAccount("deposit-idempotency-account-2")

	request := DepositRequest{
		AccountID:      accountID,
		AmountCents:    5000,
		IdempotencyKey: "deposit-idempotency-key-1",
	}

	// First request must execute the deposit.
	first, err := service.Deposit(ctx, request)
	if err != nil {
		t.Fatalf("first deposit: %v", err)
	}

	if first.StatusCode != http.StatusOK || first.Replay {
		t.Fatalf("unexpected first result: %+v", first)
	}

	// Repeating the same request must return the stored result.
	second, err := service.Deposit(ctx, request)
	if err != nil {
		t.Fatalf("replayed deposit: %v", err)
	}

	if second.StatusCode != http.StatusOK || !second.Replay {
		t.Fatalf("unexpected replay result: %+v", second)
	}

	var firstAccount, secondAccount Account

	if err := json.Unmarshal(first.Body, &firstAccount); err != nil {
		t.Fatalf("decode first response: %v", err)
	}

	if err := json.Unmarshal(second.Body, &secondAccount); err != nil {
		t.Fatalf("decode replay response: %v", err)
	}

	if firstAccount != secondAccount {
		t.Fatalf(
			"replayed response differs: first=%+v second=%+v",
			firstAccount,
			secondAccount,
		)
	}

	if secondAccount.BalanceCents != 5000 {
		t.Fatalf(
			"expected balance 5000 after replay, got %d",
			secondAccount.BalanceCents,
		)
	}

	// Same key, different amount must be rejected.
	_, err = service.Deposit(ctx, DepositRequest{
		AccountID:      accountID,
		AmountCents:    7000,
		IdempotencyKey: request.IdempotencyKey,
	})

	if !errors.Is(err, transaction.ErrIdempotencyConflict) {
		t.Fatalf("expected conflict for different amount, got %v", err)
	}

	// Same key, different account must also be rejected.
	_, err = service.Deposit(ctx, DepositRequest{
		AccountID:      otherAccountID,
		AmountCents:    5000,
		IdempotencyKey: request.IdempotencyKey,
	})

	if !errors.Is(err, transaction.ErrIdempotencyConflict) {
		t.Fatalf("expected conflict for different account, got %v", err)
	}

	// Verify that the deposit was applied exactly once.
	var balance int64

	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE id = $1`,
		accountID,
	).Scan(&balance)
	if err != nil {
		t.Fatalf("read balance: %v", err)
	}

	if balance != 5000 {
		t.Fatalf("expected persisted balance 5000, got %d", balance)
	}

	// Exactly one deposit transaction and two ledger entries.
	var depositCount, ledgerCount int

	err = pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*)
		FROM transactions
		WHERE transaction_type = 'deposit'
		`,
	).Scan(&depositCount)
	if err != nil {
		t.Fatalf("count deposits: %v", err)
	}

	err = pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*)
		FROM ledger_entries le
		JOIN transactions t ON t.id = le.transaction_id
		WHERE t.transaction_type = 'deposit'
		`,
	).Scan(&ledgerCount)
	if err != nil {
		t.Fatalf("count deposit ledger entries: %v", err)
	}

	if depositCount != 1 {
		t.Fatalf("expected 1 deposit transaction, got %d", depositCount)
	}

	if ledgerCount != 2 {
		t.Fatalf("expected 2 deposit ledger entries, got %d", ledgerCount)
	}
}
