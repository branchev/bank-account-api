package banking

import (
	"context"
	"encoding/json"
	"errors"
	accountpkg "github.com/branchev/bank-account-api/internal/account"
	"math"
	"testing"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/platform/database/testutil"
	"github.com/branchev/bank-account-api/internal/transaction"
	"github.com/jackc/pgx/v5"
)

func TestPostgresService_DepositValidation(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	ctx := context.Background()

	service := NewPostgresService(
		pool,
		accountpkg.NewPostgresRepository(),
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	created, err := service.CreateAccount(ctx, CreateAccountRequest{
		IdempotencyKey: "deposit-validation-create",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	var account accountpkg.Account
	if err := json.Unmarshal(created.Body, &account); err != nil {
		t.Fatalf("decode account: %v", err)
	}

	tests := []struct {
		name    string
		request DepositRequest
		wantErr error
	}{
		{
			name: "zero amount",
			request: DepositRequest{
				AccountID:      account.ID,
				AmountCents:    0,
				IdempotencyKey: "deposit-validation-zero",
			},
			wantErr: ErrInvalidDepositAmount,
		},
		{
			name: "negative amount",
			request: DepositRequest{
				AccountID:      account.ID,
				AmountCents:    -100,
				IdempotencyKey: "deposit-validation-negative",
			},
			wantErr: ErrInvalidDepositAmount,
		},
		{
			name: "missing idempotency key",
			request: DepositRequest{
				AccountID:   account.ID,
				AmountCents: 100,
			},
			wantErr: ErrMissingIdempotencyKey,
		},
		{
			name: "nonexistent account",
			request: DepositRequest{
				AccountID:      math.MaxInt64,
				AmountCents:    100,
				IdempotencyKey: "deposit-validation-not-found",
			},
			wantErr: pgx.ErrNoRows,
		},
		{
			name: "invalid account ID",
			request: DepositRequest{
				AccountID:      0,
				AmountCents:    100,
				IdempotencyKey: "deposit-validation-invalid-id",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.Deposit(ctx, tt.request)

			if err == nil {
				t.Fatal("expected an error")
			}

			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}

	// Rejected deposits must not change the customer balance.
	var balance int64
	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE id = $1`,
		account.ID,
	).Scan(&balance)
	if err != nil {
		t.Fatalf("read account balance: %v", err)
	}

	if balance != 0 {
		t.Fatalf("expected unchanged balance 0, got %d", balance)
	}

	// Rejected deposits must not leave committed deposit transactions.
	var depositCount int
	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM transactions WHERE transaction_type = 'deposit'`,
	).Scan(&depositCount)
	if err != nil {
		t.Fatalf("count deposits: %v", err)
	}

	if depositCount != 0 {
		t.Fatalf("expected 0 deposit transactions, got %d", depositCount)
	}
}
