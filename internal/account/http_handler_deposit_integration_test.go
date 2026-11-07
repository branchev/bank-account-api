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

func TestHTTPHandler_DepositIntegration(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	ctx := context.Background()

	service := NewPostgresService(
		pool,
		NewPostgresRepository(),
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	// Create an account with an initial balance of 100 EUR.
	created, err := service.CreateAccount(ctx, CreateAccountRequest{
		InitialBalanceCents: 10000,
		IdempotencyKey:      "http-deposit-integration-create",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	var account Account
	if err := json.Unmarshal(created.Body, &account); err != nil {
		t.Fatalf("decode created account: %v", err)
	}

	handler := NewHTTPHandler(service)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Deposit 50 EUR through the HTTP endpoint.
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/accounts/"+strconv.FormatInt(account.ID, 10)+"/deposits",
		bytes.NewBufferString(`{"amount_cents":5000}`),
	)

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "http-deposit-integration-001")

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP 200, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var updated Account
	if err := json.Unmarshal(recorder.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode deposit response: %v", err)
	}

	if updated.ID != account.ID {
		t.Fatalf("expected account ID %d, got %d", account.ID, updated.ID)
	}

	if updated.BalanceCents != 15000 {
		t.Fatalf("expected balance 15000, got %d", updated.BalanceCents)
	}

	// Verify the persisted customer balance.
	var balance int64
	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE id = $1`,
		account.ID,
	).Scan(&balance)
	if err != nil {
		t.Fatalf("read persisted balance: %v", err)
	}

	if balance != 15000 {
		t.Fatalf("expected persisted balance 15000, got %d", balance)
	}

	// Verify that the deposit has exactly two balanced ledger entries.
	var entryCount int
	var ledgerSum int64

	err = pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*), COALESCE(SUM(le.amount_cents), 0)
		FROM ledger_entries le
		JOIN transactions tr ON tr.id = le.transaction_id
		WHERE tr.idempotency_key = $1
		`,
		"http-deposit-integration-001",
	).Scan(&entryCount, &ledgerSum)
	if err != nil {
		t.Fatalf("read ledger entries: %v", err)
	}

	if entryCount != 2 || ledgerSum != 0 {
		t.Fatalf(
			"expected 2 balanced ledger entries, got count=%d sum=%d",
			entryCount,
			ledgerSum,
		)
	}
}
