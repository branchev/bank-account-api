package account

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/platform/database/testutil"
	"github.com/branchev/bank-account-api/internal/transaction"
)

func TestHTTPHandler_DepositIdempotencyIntegration(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	ctx := context.Background()

	service := NewPostgresService(
		pool,
		NewPostgresRepository(),
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	created, err := service.CreateAccount(ctx, CreateAccountRequest{
		IdempotencyKey: "http-idempotency-create-account",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	var account Account
	if err := json.Unmarshal(created.Body, &account); err != nil {
		t.Fatalf("decode account: %v", err)
	}

	handler := NewHTTPHandler(service)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	path := "/api/v1/accounts/" +
		strconv.FormatInt(account.ID, 10) +
		"/deposits"

	sendDeposit := func(amount int64) *httptest.ResponseRecorder {
		t.Helper()

		body, err := json.Marshal(struct {
			AmountCents int64 `json:"amount_cents"`
		}{
			AmountCents: amount,
		})
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}

		request := httptest.NewRequest(
			http.MethodPost,
			path,
			bytes.NewReader(body),
		)

		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "http-deposit-idempotency-001")

		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)

		return recorder
	}

	// First request executes the deposit.
	first := sendDeposit(5000)

	if first.Code != http.StatusOK {
		t.Fatalf("first deposit: expected 200, got %d: %s",
			first.Code, first.Body.String())
	}

	// Same key and payload must replay the original response.
	second := sendDeposit(5000)

	if second.Code != http.StatusOK {
		t.Fatalf("replay: expected 200, got %d: %s",
			second.Code, second.Body.String())
	}

	var firstAccount, replayedAccount Account

	if err := json.Unmarshal(first.Body.Bytes(), &firstAccount); err != nil {
		t.Fatalf("decode first response: %v", err)
	}

	if err := json.Unmarshal(second.Body.Bytes(), &replayedAccount); err != nil {
		t.Fatalf("decode replay response: %v", err)
	}

	if firstAccount != replayedAccount {
		t.Fatalf(
			"replay response differs: first=%+v replay=%+v",
			firstAccount,
			replayedAccount,
		)
	}

	// Same key with a different amount must be rejected.
	conflict := sendDeposit(7000)

	if conflict.Code != http.StatusConflict {
		t.Fatalf(
			"conflict: expected 409, got %d: %s",
			conflict.Code,
			conflict.Body.String(),
		)
	}

	// Verify that the balance was updated exactly once.
	var balance int64

	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE id = $1`,
		account.ID,
	).Scan(&balance)
	if err != nil {
		t.Fatalf("read balance: %v", err)
	}

	if balance != 5000 {
		t.Fatalf("expected balance 5000, got %d", balance)
	}

	// Exactly one deposit transaction and two ledger entries.
	var transactionCount, ledgerCount int

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM transactions WHERE transaction_type = 'deposit'`,
	).Scan(&transactionCount)
	if err != nil {
		t.Fatalf("count deposit transactions: %v", err)
	}

	err = pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*)
		FROM ledger_entries le
		JOIN transactions tr ON tr.id = le.transaction_id
		WHERE tr.transaction_type = 'deposit'
		`,
	).Scan(&ledgerCount)
	if err != nil {
		t.Fatalf("count deposit ledger entries: %v", err)
	}

	if transactionCount != 1 {
		t.Fatalf("expected 1 deposit transaction, got %d", transactionCount)
	}

	if ledgerCount != 2 {
		t.Fatalf("expected 2 deposit ledger entries, got %d", ledgerCount)
	}
}
