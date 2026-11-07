package account

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/platform/database/testutil"
	"github.com/branchev/bank-account-api/internal/transaction"
)

func TestPostgresService_CreateAccountValidation(t *testing.T) {
	tests := []struct {
		name    string
		request CreateAccountRequest
		wantErr error
	}{
		{
			name: "negative initial balance",
			request: CreateAccountRequest{
				InitialBalanceCents: -100,
				IdempotencyKey:      "negative-balance",
			},
			wantErr: ErrInvalidInitialBalance,
		},
		{
			name: "missing idempotency key",
			request: CreateAccountRequest{
				InitialBalanceCents: 10000,
				IdempotencyKey:      "",
			},
			wantErr: ErrMissingIdempotencyKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			_, err := service.CreateAccount(ctx, tt.request)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf(
					"expected error %v, got %v",
					tt.wantErr,
					err,
				)
			}

			var accountCount int
			err = pool.QueryRow(
				ctx,
				`SELECT COUNT(*) FROM accounts WHERE account_type = 'customer'`,
			).Scan(&accountCount)
			if err != nil {
				t.Fatalf("count customer accounts: %v", err)
			}

			if accountCount != 0 {
				t.Fatalf(
					"expected 0 customer accounts, got %d",
					accountCount,
				)
			}

			var transactionCount int
			err = pool.QueryRow(
				ctx,
				`SELECT COUNT(*) FROM transactions`,
			).Scan(&transactionCount)
			if err != nil {
				t.Fatalf("count transactions: %v", err)
			}

			if transactionCount != 0 {
				t.Fatalf(
					"expected 0 transactions, got %d",
					transactionCount,
				)
			}

			var ledgerCount int
			err = pool.QueryRow(
				ctx,
				`SELECT COUNT(*) FROM ledger_entries`,
			).Scan(&ledgerCount)
			if err != nil {
				t.Fatalf("count ledger entries: %v", err)
			}

			if ledgerCount != 0 {
				t.Fatalf(
					"expected 0 ledger entries, got %d",
					ledgerCount,
				)
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

			if systemBalance != 0 {
				t.Fatalf(
					"expected system balance 0, got %d",
					systemBalance,
				)
			}
		})
	}
}
