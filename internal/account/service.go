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

// Service defines the business operations for bank accounts.
type Service interface {
	CreateAccount(
		ctx context.Context,
		request CreateAccountRequest,
	) (CreateAccountResult, error)

	GetAccount(
		ctx context.Context,
		id int64,
	) (*Account, error)
}
