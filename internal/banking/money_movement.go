package banking

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalidAmount     = errors.New("amount must be positive")
	ErrSameAccount       = errors.New("source and destination accounts must differ")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrBalanceOverflow   = errors.New("account balance overflow")
	ErrBalanceUnderflow  = errors.New("account balance underflow")
)

type lockedAccount struct {
	ID           int64
	AccountType  string
	BalanceCents int64
}

// moveFunds transfers money between two accounts inside an existing
// PostgreSQL transaction.
//
// The caller owns the transaction and is responsible for committing it.
// This method must never commit or roll back the transaction itself.
func (s *PostgresService) moveFunds(
	ctx context.Context,
	tx pgx.Tx,
	transactionID int64,
	sourceAccountID int64,
	destinationAccountID int64,
	amount int64,
) (int64, error) {
	if amount <= 0 {
		return 0, ErrInvalidAmount
	}

	if sourceAccountID == destinationAccountID {
		return 0, ErrSameAccount
	}

	// Lock accounts in ascending ID order to prevent deadlocks.
	ids := []int64{sourceAccountID, destinationAccountID}

	if ids[0] > ids[1] {
		ids[0], ids[1] = ids[1], ids[0]
	}

	accounts := make(map[int64]lockedAccount, 2)

	for _, id := range ids {
		var account lockedAccount

		err := tx.QueryRow(
			ctx,
			`
			SELECT id, account_type, balance_cents
			FROM accounts
			WHERE id = $1
			FOR UPDATE
			`,
			id,
		).Scan(
			&account.ID,
			&account.AccountType,
			&account.BalanceCents,
		)
		if err != nil {
			return 0, fmt.Errorf("lock account %d: %w", id, err)
		}

		accounts[id] = account
	}

	source := accounts[sourceAccountID]
	destination := accounts[destinationAccountID]

	// Customer accounts cannot spend more than their available balance.
	if source.AccountType == "customer" && source.BalanceCents < amount {
		return 0, ErrInsufficientFunds
	}

	// Protect the source account against BIGINT underflow.
	if source.BalanceCents < math.MinInt64+amount {
		return 0, ErrBalanceUnderflow
	}

	// Protect the destination account against BIGINT overflow.
	if destination.BalanceCents > math.MaxInt64-amount {
		return 0, ErrBalanceOverflow
	}

	// Update both balances within the caller's transaction.
	_, err := tx.Exec(
		ctx,
		`
		UPDATE accounts
		SET balance_cents = balance_cents - $1
		WHERE id = $2
		`,
		amount,
		sourceAccountID,
	)
	if err != nil {
		return 0, fmt.Errorf("debit source account: %w", err)
	}

	_, err = tx.Exec(
		ctx,
		`
		UPDATE accounts
		SET balance_cents = balance_cents + $1
		WHERE id = $2
		`,
		amount,
		destinationAccountID,
	)
	if err != nil {
		return 0, fmt.Errorf("credit destination account: %w", err)
	}

	// Double-entry ledger: every movement must balance to zero.
	entries := []ledger.Entry{
		{
			TransactionID: transactionID,
			AccountID:     sourceAccountID,
			AmountCents:   -amount,
		},
		{
			TransactionID: transactionID,
			AccountID:     destinationAccountID,
			AmountCents:   amount,
		},
	}

	if err := s.ledger.CreateEntries(ctx, tx, entries); err != nil {
		return 0, fmt.Errorf("create ledger entries: %w", err)
	}

	return destination.BalanceCents + amount, nil
}
