package testutil

import (
	"context"
	"testing"
	"time"
)

func TestNewPostgresPool(t *testing.T) {
	pool := NewPostgresPool(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify that migrations created the accounts table
	// and inserted exactly one system account.
	var systemAccountCount int

	err := pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*)
		FROM accounts
		WHERE account_type = 'system'
		`,
	).Scan(&systemAccountCount)

	if err != nil {
		t.Fatalf("query system accounts: %v", err)
	}

	if systemAccountCount != 1 {
		t.Fatalf(
			"expected exactly one system account, got %d",
			systemAccountCount,
		)
	}

	// Verify that the connection uses a dedicated test schema.
	var schema string

	err = pool.QueryRow(
		ctx,
		`SELECT current_schema()`,
	).Scan(&schema)

	if err != nil {
		t.Fatalf("read current schema: %v", err)
	}

	if schema == "public" {
		t.Fatal("expected isolated schema, got public")
	}

	t.Logf("isolated schema: %s", schema)
}
