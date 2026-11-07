package banking

import (
	"context"
	"errors"
	accountpkg "github.com/branchev/bank-account-api/internal/account"
	"testing"
	"time"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/platform/database/testutil"
	"github.com/branchev/bank-account-api/internal/transaction"
)

func TestPostgresService_CreateAccountIdempotencyConflict(t *testing.T) {
	pool := testutil.NewPostgresPool(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	service := NewPostgresService(
		pool,
		accountpkg.NewPostgresRepository(),
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	key := "account-conflict-test"

	// First request successfully creates an account.
	_, err := service.CreateAccount(ctx, CreateAccountRequest{
		InitialBalanceCents: 10000,
		IdempotencyKey:      key,
	})
	if err != nil {
		t.Fatalf("first request: %v", err)
	}

	// Capture financial state before the conflicting request.
	var accountCountBefore int
	var transactionCountBefore int
	var ledgerCountBefore int
	var systemBalanceBefore int64

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM accounts WHERE account_type = 'customer'`,
	).Scan(&accountCountBefore)
	if err != nil {
		t.Fatalf("count accounts before conflict: %v", err)
	}

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM transactions`,
	).Scan(&transactionCountBefore)
	if err != nil {
		t.Fatalf("count transactions before conflict: %v", err)
	}

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM ledger_entries`,
	).Scan(&ledgerCountBefore)
	if err != nil {
		t.Fatalf("count ledger entries before conflict: %v", err)
	}

	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE account_type = 'system'`,
	).Scan(&systemBalanceBefore)
	if err != nil {
		t.Fatalf("read system balance before conflict: %v", err)
	}

	// Same key with a different payload must be rejected.
	_, err = service.CreateAccount(ctx, CreateAccountRequest{
		InitialBalanceCents: 20000,
		IdempotencyKey:      key,
	})

	if !errors.Is(err, transaction.ErrIdempotencyConflict) {
		t.Fatalf(
			"expected ErrIdempotencyConflict, got %v",
			err,
		)
	}

	// Verify the conflicting request made no database changes.
	var accountCountAfter int
	var transactionCountAfter int
	var ledgerCountAfter int
	var systemBalanceAfter int64

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM accounts WHERE account_type = 'customer'`,
	).Scan(&accountCountAfter)
	if err != nil {
		t.Fatalf("count accounts after conflict: %v", err)
	}

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM transactions`,
	).Scan(&transactionCountAfter)
	if err != nil {
		t.Fatalf("count transactions after conflict: %v", err)
	}

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM ledger_entries`,
	).Scan(&ledgerCountAfter)
	if err != nil {
		t.Fatalf("count ledger entries after conflict: %v", err)
	}

	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE account_type = 'system'`,
	).Scan(&systemBalanceAfter)
	if err != nil {
		t.Fatalf("read system balance after conflict: %v", err)
	}

	if accountCountAfter != accountCountBefore {
		t.Fatalf(
			"account count changed: before=%d after=%d",
			accountCountBefore,
			accountCountAfter,
		)
	}

	if transactionCountAfter != transactionCountBefore {
		t.Fatalf(
			"transaction count changed: before=%d after=%d",
			transactionCountBefore,
			transactionCountAfter,
		)
	}

	if ledgerCountAfter != ledgerCountBefore {
		t.Fatalf(
			"ledger count changed: before=%d after=%d",
			ledgerCountBefore,
			ledgerCountAfter,
		)
	}

	if systemBalanceAfter != systemBalanceBefore {
		t.Fatalf(
			"system balance changed: before=%d after=%d",
			systemBalanceBefore,
			systemBalanceAfter,
		)
	}
}
