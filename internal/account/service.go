package account

import (
	"context"
	"errors"
)

var (
	ErrInvalidInitialBalance = errors.New(
		"initial balance cannot be negative",
	)

	ErrMissingIdempotencyKey = errors.New(
		"idempotency key is required",
	)

	ErrInvalidDepositAmount = errors.New(
		"deposit amount must be positive",
	)
)

// CreateAccountRequest contains the data needed to open a new account.
type CreateAccountRequest struct {
	InitialBalanceCents int64
	IdempotencyKey      string
}

// CreateAccountResult contains the HTTP response data
// and indicates whether it is a replay of an earlier request.
type CreateAccountResult struct {
	StatusCode int
	Body       []byte
	Replay     bool
}

// DepositRequest contains the data needed to deposit money
// into an existing customer account.
type DepositRequest struct {
	AccountID      int64
	AmountCents    int64
	IdempotencyKey string
}

// DepositResult contains the stored HTTP response
// and indicates whether the operation was replayed.
type DepositResult struct {
	StatusCode int
	Body       []byte
	Replay     bool
}

type Service interface {
	CreateAccount(
		ctx context.Context,
		request CreateAccountRequest,
	) (CreateAccountResult, error)

	GetAccount(
		ctx context.Context,
		id int64,
	) (*Account, error)

	Deposit(
		ctx context.Context,
		request DepositRequest,
	) (DepositResult, error)
}
