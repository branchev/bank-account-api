package transaction

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestTryCreate_NewKey verifies that a new idempotency key
// is successfully reserved.
func TestTryCreate_NewKey(t *testing.T) {
	ctx, tx := setupTestTransaction(t)
	repo := NewPostgresRepository()

	record := &Transaction{
		Type:           TypeDeposit,
		IdempotencyKey: fmt.Sprintf("try-create-new-%s", t.Name()),
		RequestHash:    "request-hash",
	}

	created, err := repo.TryCreate(ctx, tx, record)
	if err != nil {
		t.Fatalf("try create transaction: %v", err)
	}

	if !created {
		t.Fatal("expected a new transaction to be created")
	}

	if record.ID <= 0 {
		t.Fatalf("expected positive transaction ID, got %d", record.ID)
	}
}

// TestTryCreate_ExistingKey verifies that the same key
// cannot reserve a second transaction.
func TestTryCreate_ExistingKey(t *testing.T) {
	ctx, tx := setupTestTransaction(t)
	repo := NewPostgresRepository()

	key := fmt.Sprintf("try-create-existing-%s", t.Name())

	first := &Transaction{
		Type:           TypeDeposit,
		IdempotencyKey: key,
		RequestHash:    "original-hash",
	}

	created, err := repo.TryCreate(ctx, tx, first)
	if err != nil || !created {
		t.Fatalf("first reservation failed: created=%v, err=%v", created, err)
	}

	second := &Transaction{
		Type:           TypeDeposit,
		IdempotencyKey: key,
		RequestHash:    "different-hash",
	}

	created, err = repo.TryCreate(ctx, tx, second)
	if err != nil {
		t.Fatalf("second reservation returned error: %v", err)
	}

	if created {
		t.Fatal("expected duplicate key to be rejected")
	}

	existing, err := repo.GetByIdempotencyKey(ctx, tx, key)
	if err != nil {
		t.Fatalf("get existing transaction: %v", err)
	}

	if existing.ID != first.ID {
		t.Fatalf("expected original transaction ID %d, got %d", first.ID, existing.ID)
	}
}

// TestTryCreate_ConcurrentRequests verifies that PostgreSQL
// serializes concurrent reservations of the same idempotency key.
func TestTryCreate_ConcurrentRequests(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	repo := NewPostgresRepository()
	key := fmt.Sprintf("try-create-concurrent-%s-%d", t.Name(), time.Now().UnixNano())

	// Transaction A reserves the key but does not commit yet.
	txA, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction A: %v", err)
	}
	defer txA.Rollback(context.Background())

	recordA := &Transaction{
		Type:           TypeTransfer,
		IdempotencyKey: key,
		RequestHash:    "same-request",
	}

	created, err := repo.TryCreate(ctx, txA, recordA)
	if err != nil || !created {
		t.Fatalf("transaction A reservation failed: created=%v, err=%v", created, err)
	}

	// Transaction B attempts to reserve the same key concurrently.
	txB, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction B: %v", err)
	}
	defer txB.Rollback(context.Background())

	type result struct {
		created bool
		err     error
	}

	resultCh := make(chan result, 1)

	go func() {
		recordB := &Transaction{
			Type:           TypeTransfer,
			IdempotencyKey: key,
			RequestHash:    "same-request",
		}

		created, err := repo.TryCreate(ctx, txB, recordB)
		resultCh <- result{created: created, err: err}
	}()

	// B must not finish while A still holds the uncommitted key.
	select {
	case result := <-resultCh:
		t.Fatalf("transaction B finished before A committed: %+v", result)
	case <-time.After(150 * time.Millisecond):
		// Expected: B is waiting for A.
	}

	// Complete A's reservation and commit.
	if err := repo.Complete(
		ctx,
		txA,
		recordA.ID,
		201,
		[]byte(`{"result":"success"}`),
	); err != nil {
		t.Fatalf("complete transaction A: %v", err)
	}

	if err := txA.Commit(ctx); err != nil {
		t.Fatalf("commit transaction A: %v", err)
	}

	// B should now discover that the key already exists.
	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("transaction B returned error: %v", result.err)
		}

		if result.created {
			t.Fatal("transaction B must not create a duplicate")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for transaction B")
	}

	// Under READ COMMITTED, B can now read the committed response.
	existing, err := repo.GetByIdempotencyKey(ctx, txB, key)
	if err != nil {
		t.Fatalf("get existing transaction from B: %v", err)
	}

	replay, err := ValidateReplay(existing, TypeTransfer, "same-request")
	if err != nil {
		t.Fatalf("validate replay: %v", err)
	}

	if !replay.Replay || replay.StatusCode != 201 {
		t.Fatalf("unexpected replay result: %+v", replay)
	}

	// Clean up the committed test record.
	if err := txB.Rollback(ctx); err != nil {
		t.Fatalf("rollback transaction B: %v", err)
	}

	_, err = pool.Exec(ctx, "DELETE FROM transactions WHERE id = $1", recordA.ID)
	if err != nil {
		t.Fatalf("delete test transaction: %v", err)
	}
}

func TestTryCreate_ConcurrentRollback(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	repo := NewPostgresRepository()
	key := fmt.Sprintf(
		"try-create-rollback-%s-%d",
		t.Name(),
		time.Now().UnixNano(),
	)

	txA, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction A: %v", err)
	}
	defer txA.Rollback(context.Background())

	recordA := &Transaction{
		Type:           TypeTransfer,
		IdempotencyKey: key,
		RequestHash:    "same-request",
	}

	created, err := repo.TryCreate(ctx, txA, recordA)
	if err != nil || !created {
		t.Fatalf("A reservation failed: created=%v, err=%v", created, err)
	}

	txB, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction B: %v", err)
	}
	defer txB.Rollback(context.Background())

	type result struct {
		created bool
		err     error
	}

	resultCh := make(chan result, 1)

	go func() {
		recordB := &Transaction{
			Type:           TypeTransfer,
			IdempotencyKey: key,
			RequestHash:    "same-request",
		}

		created, err := repo.TryCreate(ctx, txB, recordB)
		resultCh <- result{created: created, err: err}
	}()

	select {
	case result := <-resultCh:
		t.Fatalf("B finished before A rolled back: %+v", result)
	case <-time.After(150 * time.Millisecond):
		// Expected: B is waiting for A.
	}

	// A fails and releases its reservation.
	if err := txA.Rollback(ctx); err != nil {
		t.Fatalf("rollback transaction A: %v", err)
	}

	// B should now successfully reserve the same key.
	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("B reservation failed: %v", result.err)
		}

		if !result.created {
			t.Fatal("expected B to reserve the key after A rolled back")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for transaction B")
	}

	// B's reservation is rolled back during cleanup.
}
