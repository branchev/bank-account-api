package transaction

import (
	"errors"
	"net/http"
	"testing"
)

func TestValidateReplay_Success(t *testing.T) {
	status := http.StatusCreated
	body := []byte(`{"id":42,"balance_cents":10000}`)

	existing := &Transaction{
		Type:           TypeAccountCreation,
		IdempotencyKey: "test-key",
		RequestHash:    "original-hash",
		ResponseStatus: &status,
		ResponseBody:   body,
	}

	result, err := ValidateReplay(
		existing,
		TypeAccountCreation,
		"original-hash",
	)
	if err != nil {
		t.Fatalf("expected successful replay, got: %v", err)
	}

	if !result.Replay {
		t.Fatal("expected Replay to be true")
	}

	if result.StatusCode != http.StatusCreated {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusCreated,
			result.StatusCode,
		)
	}

	if string(result.Body) != string(body) {
		t.Fatalf(
			"expected body %s, got %s",
			body,
			result.Body,
		)
	}
}

func TestValidateReplay_DifferentRequestHash(t *testing.T) {
	status := http.StatusCreated

	existing := &Transaction{
		Type:           TypeAccountCreation,
		RequestHash:    "original-hash",
		ResponseStatus: &status,
		ResponseBody:   []byte(`{"id":42}`),
	}

	_, err := ValidateReplay(
		existing,
		TypeAccountCreation,
		"different-hash",
	)

	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf(
			"expected ErrIdempotencyConflict, got %v",
			err,
		)
	}
}

func TestValidateReplay_DifferentTransactionType(t *testing.T) {
	status := http.StatusCreated

	existing := &Transaction{
		Type:           TypeDeposit,
		RequestHash:    "same-hash",
		ResponseStatus: &status,
		ResponseBody:   []byte(`{"id":42}`),
	}

	_, err := ValidateReplay(
		existing,
		TypeTransfer,
		"same-hash",
	)

	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf(
			"expected ErrIdempotencyConflict, got %v",
			err,
		)
	}
}

func TestValidateReplay_IncompleteTransaction(t *testing.T) {
	existing := &Transaction{
		Type:        TypeDeposit,
		RequestHash: "same-hash",
	}

	_, err := ValidateReplay(
		existing,
		TypeDeposit,
		"same-hash",
	)

	if err == nil {
		t.Fatal("expected error for incomplete transaction")
	}

	if errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal("incomplete transaction must not be treated as a key conflict")
	}
}

func TestValidateReplay_NilTransaction(t *testing.T) {
	_, err := ValidateReplay(
		nil,
		TypeDeposit,
		"some-hash",
	)

	if err == nil {
		t.Fatal("expected error for nil transaction")
	}
}

func TestConflictStatusCode(t *testing.T) {
	if got := ConflictStatusCode(); got != http.StatusConflict {
		t.Fatalf(
			"expected HTTP %d, got %d",
			http.StatusConflict,
			got,
		)
	}
}
