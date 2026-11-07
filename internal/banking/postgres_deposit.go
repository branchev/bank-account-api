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

// Deposit adds funds to an existing customer account atomically.
func (s *PostgresService) Deposit(
	ctx context.Context,
	request DepositRequest,
) (DepositResult, error) {
	if request.IdempotencyKey == "" {
		return DepositResult{}, ErrMissingIdempotencyKey
	}

	if request.AmountCents <= 0 {
		return DepositResult{}, ErrInvalidDepositAmount
	}

	if request.AccountID <= 0 {
		return DepositResult{}, fmt.Errorf("invalid account ID: %d", request.AccountID)
	}

	requestHash := hashDepositRequest(request.AccountID, request.AmountCents)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.ReadCommitted,
	})
	if err != nil {
		return DepositResult{}, fmt.Errorf("begin deposit transaction: %w", err)
	}
	defer tx.Rollback(context.Background())

	record := &transaction.Transaction{
		Type:           transaction.TypeDeposit,
		IdempotencyKey: request.IdempotencyKey,
		RequestHash:    requestHash,
	}

	created, err := s.transactions.TryCreate(ctx, tx, record)
	if err != nil {
		return DepositResult{}, fmt.Errorf("reserve deposit idempotency key: %w", err)
	}

	if !created {
		existing, err := s.transactions.GetByIdempotencyKey(
			ctx,
			tx,
			request.IdempotencyKey,
		)
		if err != nil {
			return DepositResult{}, fmt.Errorf("get existing transaction: %w", err)
		}

		replay, err := transaction.ValidateReplay(
			existing,
			transaction.TypeDeposit,
			requestHash,
		)
		if err != nil {
			return DepositResult{}, err
		}

		return DepositResult{
			StatusCode: replay.StatusCode,
			Body:       replay.Body,
			Replay:     true,
		}, nil
	}

	// Only customer accounts can receive deposits.
	account, err := s.accounts.GetByID(ctx, tx, request.AccountID)
	if err != nil {
		return DepositResult{}, fmt.Errorf("get deposit account: %w", err)
	}

	var systemAccountID int64

	err = tx.QueryRow(
		ctx,
		`SELECT id FROM accounts WHERE account_type = 'system'`,
	).Scan(&systemAccountID)
	if err != nil {
		return DepositResult{}, fmt.Errorf("find system account: %w", err)
	}

	newBalance, err := s.moveFunds(
		ctx,
		tx,
		record.ID,
		systemAccountID,
		account.ID,
		request.AmountCents,
	)
	if err != nil {
		return DepositResult{}, fmt.Errorf("move deposit funds: %w", err)
	}

	account.BalanceCents = newBalance

	responseBody, err := json.Marshal(account)
	if err != nil {
		return DepositResult{}, fmt.Errorf("marshal deposit response: %w", err)
	}

	if err := s.transactions.Complete(
		ctx,
		tx,
		record.ID,
		http.StatusOK,
		responseBody,
	); err != nil {
		return DepositResult{}, fmt.Errorf("complete deposit transaction: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return DepositResult{}, fmt.Errorf("commit deposit transaction: %w", err)
	}

	return DepositResult{
		StatusCode: http.StatusOK,
		Body:       responseBody,
		Replay:     false,
	}, nil
}

// hashDepositRequest includes both the destination account and amount.
// This prevents reuse of an idempotency key with different parameters.
func hashDepositRequest(accountID, amountCents int64) string {
	payload := fmt.Sprintf("deposit:%d:%d", accountID, amountCents)

	sum := sha256.Sum256([]byte(payload))

	return hex.EncodeToString(sum[:])
}
