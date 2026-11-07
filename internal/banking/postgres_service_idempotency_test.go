package banking

import (
	"context"
	"encoding/json"
	accountpkg "github.com/branchev/bank-account-api/internal/account"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/platform/database/testutil"
	"github.com/branchev/bank-account-api/internal/transaction"
)

func TestPostgresService_CreateAccountIdempotency(t *testing.T) {
	pool := testutil.NewPostgresPool(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	service := NewPostgresService(
		pool,
		accountpkg.NewPostgresRepository(),
		ledger.NewPostgresRepository(),
		transaction.NewPostgresRepository(),
	)

	request := CreateAccountRequest{
		InitialBalanceCents: 10000,
		IdempotencyKey:      "account-idempotency-test",
	}

	first, err := service.CreateAccount(ctx, request)
	if err != nil {
		t.Fatalf("first request: %v", err)
	}

	if first.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", first.StatusCode)
	}

	if first.Replay {
		t.Fatal("first request must not be a replay")
	}

	var accountCountBefore int
	var ledgerCountBefore int
	var systemBalanceBefore int64

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM accounts WHERE account_type = 'customer'`,
	).Scan(&accountCountBefore)
	if err != nil {
		t.Fatalf("count accounts before replay: %v", err)
	}

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM ledger_entries`,
	).Scan(&ledgerCountBefore)
	if err != nil {
		t.Fatalf("count ledger entries before replay: %v", err)
	}

	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE account_type = 'system'`,
	).Scan(&systemBalanceBefore)
	if err != nil {
		t.Fatalf("read system balance before replay: %v", err)
	}

	second, err := service.CreateAccount(ctx, request)
	if err != nil {
		t.Fatalf("second request: %v", err)
	}

	if !second.Replay {
		t.Fatal("second request must be a replay")
	}

	if second.StatusCode != first.StatusCode {
		t.Fatalf(
			"expected status %d, got %d",
			first.StatusCode,
			second.StatusCode,
		)
	}

	var firstJSON any
	var secondJSON any

	if err := json.Unmarshal(first.Body, &firstJSON); err != nil {
		t.Fatalf("decode first response: %v", err)
	}

	if err := json.Unmarshal(second.Body, &secondJSON); err != nil {
		t.Fatalf("decode replay response: %v", err)
	}

	if !reflect.DeepEqual(firstJSON, secondJSON) {
		t.Fatalf(
			"response mismatch:\nfirst: %s\nsecond: %s",
			first.Body,
			second.Body,
		)
	}

	var accountCountAfter int
	var ledgerCountAfter int
	var systemBalanceAfter int64

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM accounts WHERE account_type = 'customer'`,
	).Scan(&accountCountAfter)
	if err != nil {
		t.Fatalf("count accounts after replay: %v", err)
	}

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM ledger_entries`,
	).Scan(&ledgerCountAfter)
	if err != nil {
		t.Fatalf("count ledger entries after replay: %v", err)
	}

	err = pool.QueryRow(
		ctx,
		`SELECT balance_cents FROM accounts WHERE account_type = 'system'`,
	).Scan(&systemBalanceAfter)
	if err != nil {
		t.Fatalf("read system balance after replay: %v", err)
	}

	if accountCountAfter != accountCountBefore {
		t.Fatalf(
			"account count changed: before=%d after=%d",
			accountCountBefore,
			accountCountAfter,
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
