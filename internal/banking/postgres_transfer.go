package banking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/branchev/bank-account-api/internal/transaction"
	"github.com/jackc/pgx/v5"
)

// Transfer moves funds atomically between two customer accounts.
func (s *PostgresService) Transfer(
	ctx context.Context,
	request TransferRequest,
) (TransferResult, error) {
	if request.IdempotencyKey == "" {
		return TransferResult{}, ErrMissingIdempotencyKey
	}

	if request.AmountCents <= 0 {
		return TransferResult{}, ErrInvalidTransferAmount
	}

	if request.SourceAccountID <= 0 || request.DestinationAccountID <= 0 {
		return TransferResult{}, fmt.Errorf("invalid account ID")
	}

	if request.SourceAccountID == request.DestinationAccountID {
		return TransferResult{}, ErrSameAccount
	}

	requestHash := hashTransferRequest(request)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.ReadCommitted,
	})
	if err != nil {
		return TransferResult{}, fmt.Errorf("begin transfer transaction: %w", err)
	}
	defer tx.Rollback(context.Background())

	record := &transaction.Transaction{
		Type:           transaction.TypeTransfer,
		IdempotencyKey: request.IdempotencyKey,
		RequestHash:    requestHash,
	}

	created, err := s.transactions.TryCreate(ctx, tx, record)
	if err != nil {
		return TransferResult{}, fmt.Errorf("reserve transfer idempotency key: %w", err)
	}

	if !created {
		existing, err := s.transactions.GetByIdempotencyKey(
			ctx,
			tx,
			request.IdempotencyKey,
		)
		if err != nil {
			return TransferResult{}, fmt.Errorf("get existing transaction: %w", err)
		}

		replay, err := transaction.ValidateReplay(
			existing,
			transaction.TypeTransfer,
			requestHash,
		)
		if err != nil {
			return TransferResult{}, err
		}

		return TransferResult{
			StatusCode: replay.StatusCode,
			Body:       replay.Body,
			Replay:     true,
		}, nil
	}

	// Both accounts must exist and must be customer accounts.
	if _, err := s.accounts.GetByID(ctx, tx, request.SourceAccountID); err != nil {
		return TransferResult{}, fmt.Errorf("get source account: %w", err)
	}

	if _, err := s.accounts.GetByID(ctx, tx, request.DestinationAccountID); err != nil {
		return TransferResult{}, fmt.Errorf("get destination account: %w", err)
	}

	newBalance, err := s.moveFunds(
		ctx,
		tx,
		record.ID,
		request.SourceAccountID,
		request.DestinationAccountID,
		request.AmountCents,
	)
	if err != nil {
		return TransferResult{}, fmt.Errorf("move transfer funds: %w", err)
	}

	responseBody, err := json.Marshal(struct {
		TransactionID        int64 `json:"transaction_id"`
		SourceAccountID      int64 `json:"source_account_id"`
		DestinationAccountID int64 `json:"destination_account_id"`
		AmountCents          int64 `json:"amount_cents"`
		DestinationBalance   int64 `json:"destination_balance_cents"`
	}{
		TransactionID:        record.ID,
		SourceAccountID:      request.SourceAccountID,
		DestinationAccountID: request.DestinationAccountID,
		AmountCents:          request.AmountCents,
		DestinationBalance:   newBalance,
	})
	if err != nil {
		return TransferResult{}, fmt.Errorf("marshal transfer response: %w", err)
	}

	if err := s.transactions.Complete(
		ctx,
		tx,
		record.ID,
		http.StatusOK,
		responseBody,
	); err != nil {
		return TransferResult{}, fmt.Errorf("complete transfer transaction: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return TransferResult{}, fmt.Errorf("commit transfer transaction: %w", err)
	}

	return TransferResult{
		StatusCode: http.StatusOK,
		Body:       responseBody,
		Replay:     false,
	}, nil
}

// hashTransferRequest includes all financial parameters.
func hashTransferRequest(request TransferRequest) string {
	payload := fmt.Sprintf(
		"transfer:%d:%d:%d",
		request.SourceAccountID,
		request.DestinationAccountID,
		request.AmountCents,
	)

	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
