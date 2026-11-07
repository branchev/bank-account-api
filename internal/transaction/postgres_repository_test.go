package transaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupTestTransaction(t *testing.T) (context.Context, pgx.Tx) {
	t.Helper()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create database pool: %v", err)
	}
	t.Cleanup(pool.Close)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin database transaction: %v", err)
	}

	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rollback test transaction: %v", err)
		}
	})

	return ctx, tx
}

func newTestTransaction(t *testing.T) *Transaction {
	t.Helper()

	return &Transaction{
		Type:           TypeDeposit,
		IdempotencyKey: fmt.Sprintf("transaction-test-%s", t.Name()),
		RequestHash:    "test-request-hash",
	}
}

func TestPostgresRepository_Create(t *testing.T) {
	ctx, tx := setupTestTransaction(t)
	repo := NewPostgresRepository()

	record := newTestTransaction(t)

	if err := repo.Create(ctx, tx, record); err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	if record.ID <= 0 {
		t.Fatalf("expected positive transaction ID, got %d", record.ID)
	}

	var count int
	err := tx.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM transactions WHERE id = $1",
		record.ID,
	).Scan(&count)

	if err != nil {
		t.Fatalf("count transactions: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected one transaction, got %d", count)
	}
}

func TestPostgresRepository_GetByIdempotencyKey(t *testing.T) {
	ctx, tx := setupTestTransaction(t)
	repo := NewPostgresRepository()

	record := newTestTransaction(t)

	if err := repo.Create(ctx, tx, record); err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	found, err := repo.GetByIdempotencyKey(
		ctx,
		tx,
		record.IdempotencyKey,
	)
	if err != nil {
		t.Fatalf("get transaction: %v", err)
	}

	if found.ID != record.ID ||
		found.Type != record.Type ||
		found.IdempotencyKey != record.IdempotencyKey ||
		found.RequestHash != record.RequestHash {
		t.Fatalf("unexpected transaction: %+v", found)
	}

	if found.ResponseStatus != nil {
		t.Fatalf("expected nil response status, got %d", *found.ResponseStatus)
	}

	if found.ResponseBody != nil {
		t.Fatalf("expected nil response body, got %s", found.ResponseBody)
	}
}

func TestPostgresRepository_GetByIdempotencyKey_NotFound(t *testing.T) {
	ctx, tx := setupTestTransaction(t)
	repo := NewPostgresRepository()

	_, err := repo.GetByIdempotencyKey(
		ctx,
		tx,
		"non-existent-idempotency-key",
	)

	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}
}

func TestPostgresRepository_Complete(t *testing.T) {
	ctx, tx := setupTestTransaction(t)
	repo := NewPostgresRepository()

	record := newTestTransaction(t)

	if err := repo.Create(ctx, tx, record); err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	responseBody := []byte(`{"id":42,"balance_cents":10000}`)

	if err := repo.Complete(
		ctx,
		tx,
		record.ID,
		201,
		responseBody,
	); err != nil {
		t.Fatalf("complete transaction: %v", err)
	}

	found, err := repo.GetByIdempotencyKey(
		ctx,
		tx,
		record.IdempotencyKey,
	)
	if err != nil {
		t.Fatalf("get completed transaction: %v", err)
	}

	if found.ResponseStatus == nil || *found.ResponseStatus != 201 {
		t.Fatalf("expected response status 201, got %v", found.ResponseStatus)
	}

	assertJSONEqual(t, responseBody, found.ResponseBody)
}

func TestPostgresRepository_CompleteTwice(t *testing.T) {
	ctx, tx := setupTestTransaction(t)
	repo := NewPostgresRepository()

	record := newTestTransaction(t)

	if err := repo.Create(ctx, tx, record); err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	if err := repo.Complete(
		ctx,
		tx,
		record.ID,
		201,
		[]byte(`{"result":"original"}`),
	); err != nil {
		t.Fatalf("first completion: %v", err)
	}

	err := repo.Complete(
		ctx,
		tx,
		record.ID,
		200,
		[]byte(`{"result":"overwritten"}`),
	)

	if err == nil {
		t.Fatal("expected error when completing a transaction twice")
	}

	found, err := repo.GetByIdempotencyKey(
		ctx,
		tx,
		record.IdempotencyKey,
	)
	if err != nil {
		t.Fatalf("get transaction: %v", err)
	}

	if found.ResponseStatus == nil || *found.ResponseStatus != 201 {
		t.Fatalf("original response status was modified")
	}

	assertJSONEqual(
		t,
		[]byte(`{"result":"original"}`),
		found.ResponseBody,
	)
}

func TestPostgresRepository_DuplicateIdempotencyKey(t *testing.T) {
	ctx, tx := setupTestTransaction(t)
	repo := NewPostgresRepository()

	first := newTestTransaction(t)

	if err := repo.Create(ctx, tx, first); err != nil {
		t.Fatalf("create first transaction: %v", err)
	}

	duplicate := &Transaction{
		Type:           TypeTransfer,
		IdempotencyKey: first.IdempotencyKey,
		RequestHash:    "different-request-hash",
	}

	// A SAVEPOINT allows us to recover from PostgreSQL's unique
	// constraint violation without aborting the outer test transaction.
	_, err := tx.Exec(ctx, "SAVEPOINT before_duplicate")
	if err != nil {
		t.Fatalf("create savepoint: %v", err)
	}

	err = repo.Create(ctx, tx, duplicate)
	if err == nil {
		t.Fatal("expected duplicate idempotency key to be rejected")
	}

	_, rollbackErr := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT before_duplicate")
	if rollbackErr != nil {
		t.Fatalf("rollback to savepoint: %v", rollbackErr)
	}

	var count int
	err = tx.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM transactions WHERE idempotency_key = $1",
		first.IdempotencyKey,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count transactions: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected exactly one transaction, got %d", count)
	}
}

func TestPostgresRepository_Rollback(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create database pool: %v", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	repo := NewPostgresRepository()
	record := newTestTransaction(t)

	if err := repo.Create(ctx, tx, record); err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback transaction: %v", err)
	}

	var count int
	err = pool.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM transactions WHERE idempotency_key = $1",
		record.IdempotencyKey,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count transactions after rollback: %v", err)
	}

	if count != 0 {
		t.Fatalf("expected no persisted transaction after rollback, got %d", count)
	}
}

func assertJSONEqual(t *testing.T, expected, actual []byte) {
	t.Helper()

	var expectedJSON any
	var actualJSON any

	if err := json.Unmarshal(expected, &expectedJSON); err != nil {
		t.Fatalf("invalid expected JSON: %v", err)
	}

	if err := json.Unmarshal(actual, &actualJSON); err != nil {
		t.Fatalf("invalid actual JSON: %v", err)
	}

	if !reflect.DeepEqual(expectedJSON, actualJSON) {
		t.Fatalf("JSON mismatch:\nexpected: %s\nactual: %s", expected, actual)
	}
}
