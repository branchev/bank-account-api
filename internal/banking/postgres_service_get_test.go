package banking

import (
	"context"
	"encoding/json"
	accountpkg "github.com/branchev/bank-account-api/internal/account"
	"testing"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/platform/database/testutil"
	"github.com/branchev/bank-account-api/internal/transaction"
)

func TestPostgresService_GetAccount(t *testing.T) {
	ctx := context.Background()

	pool := testutil.NewPostgresPool(t)
	defer pool.Close()

	accountRepo := accountpkg.NewPostgresRepository()

	service := NewPostgresService(
		pool,
		accountRepo,
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	// Create an account through the existing business workflow.
	created, err := service.CreateAccount(
		ctx,
		CreateAccountRequest{
			InitialBalanceCents: 10000,
			IdempotencyKey:      "get-account-test",
		},
	)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// Retrieve the created account ID from the stored response.
	var createdAccount accountpkg.Account

	if err := json.Unmarshal(created.Body, &createdAccount); err != nil {
		t.Fatalf("decode created account: %v", err)
	}

	t.Run("existing account", func(t *testing.T) {
		account, err := service.GetAccount(ctx, createdAccount.ID)
		if err != nil {
			t.Fatalf("get account: %v", err)
		}

		if account.ID != createdAccount.ID {
			t.Errorf(
				"expected account ID %d, got %d",
				createdAccount.ID,
				account.ID,
			)
		}

		if account.Currency != "EUR" {
			t.Errorf("expected EUR, got %s", account.Currency)
		}

		if account.BalanceCents != 10000 {
			t.Errorf(
				"expected balance 10000, got %d",
				account.BalanceCents,
			)
		}
	})

	t.Run("nonexistent account", func(t *testing.T) {
		account, err := service.GetAccount(ctx, -1)

		if err == nil {
			t.Fatal("expected an error for nonexistent account")
		}

		if account != nil {
			t.Fatalf("expected nil account, got %+v", account)
		}
	})
}
