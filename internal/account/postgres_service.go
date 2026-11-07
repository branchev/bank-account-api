package account

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/transaction"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresService coordinates account creation in one database transaction.
type PostgresService struct {
	pool         *pgxpool.Pool
	accounts     Repository
	ledger       ledger.Repository
	transactions transaction.Repository
}

func NewPostgresService(
	pool *pgxpool.Pool,
	accounts Repository,
	ledgerRepo ledger.Repository,
	transactionRepo transaction.Repository,
) *PostgresService {
	return &PostgresService{
		pool:         pool,
		accounts:     accounts,
		ledger:       ledgerRepo,
		transactions: transactionRepo,
	}
}

// CreateAccount creates a customer account and optionally funds it
// through balanced ledger entries in a single PostgreSQL transaction.
func (s *PostgresService) CreateAccount(
	ctx context.Context,
	request CreateAccountRequest,
) (CreateAccountResult, error) {
	if request.IdempotencyKey == "" {
		return CreateAccountResult{}, ErrMissingIdempotencyKey
	}

	if request.InitialBalanceCents < 0 {
		return CreateAccountResult{}, ErrInvalidInitialBalance
	}

	requestHash := hashCreateAccountRequest(request.InitialBalanceCents)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.ReadCommitted,
	})
	if err != nil {
		return CreateAccountResult{}, fmt.Errorf("begin account transaction: %w", err)
	}
	defer tx.Rollback(context.Background())

	record := &transaction.Transaction{
		Type:           transaction.TypeAccountCreation,
		IdempotencyKey: request.IdempotencyKey,
		RequestHash:    requestHash,
	}

	created, err := s.transactions.TryCreate(ctx, tx, record)
	if err != nil {
		return CreateAccountResult{}, fmt.Errorf("reserve idempotency key: %w", err)
	}

	if !created {
		existing, err := s.transactions.GetByIdempotencyKey(
			ctx,
			tx,
			request.IdempotencyKey,
		)
		if err != nil {
			return CreateAccountResult{}, fmt.Errorf("get existing transaction: %w", err)
		}

		replay, err := transaction.ValidateReplay(
			existing,
			transaction.TypeAccountCreation,
			requestHash,
		)
		if err != nil {
			return CreateAccountResult{}, err
		}

		return CreateAccountResult{
			StatusCode: replay.StatusCode,
			Body:       replay.Body,
			Replay:     true,
		}, nil
	}

	account := &Account{
		Currency:     "EUR",
		BalanceCents: 0,
	}

	if err := s.accounts.Create(ctx, tx, account); err != nil {
		return CreateAccountResult{}, fmt.Errorf("create account: %w", err)
	}

	if request.InitialBalanceCents > 0 {
		if err := s.fundInitialBalance(
			ctx,
			tx,
			record.ID,
			account,
			request.InitialBalanceCents,
		); err != nil {
			return CreateAccountResult{}, err
		}
	}

	responseBody, err := json.Marshal(account)
	if err != nil {
		return CreateAccountResult{}, fmt.Errorf("marshal account response: %w", err)
	}

	if err := s.transactions.Complete(
		ctx,
		tx,
		record.ID,
		http.StatusCreated,
		responseBody,
	); err != nil {
		return CreateAccountResult{}, fmt.Errorf("complete transaction: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return CreateAccountResult{}, fmt.Errorf("commit account transaction: %w", err)
	}

	return CreateAccountResult{
		StatusCode: http.StatusCreated,
		Body:       responseBody,
		Replay:     false,
	}, nil
}

func (s *PostgresService) fundInitialBalance(
	ctx context.Context,
	tx pgx.Tx,
	transactionID int64,
	account *Account,
	amount int64,
) error {
	// Find the system funding account.
	var systemAccountID int64

	err := tx.QueryRow(
		ctx,
		`SELECT id FROM accounts WHERE account_type = 'system'`,
	).Scan(&systemAccountID)
	if err != nil {
		return fmt.Errorf("find system account: %w", err)
	}

	// Move funds atomically from the system account to the customer.
	newBalance, err := s.moveFunds(
		ctx,
		tx,
		transactionID,
		systemAccountID,
		account.ID,
		amount,
	)
	if err != nil {
		return fmt.Errorf("fund initial balance: %w", err)
	}

	account.BalanceCents = newBalance

	return nil
}

func hashCreateAccountRequest(initialBalanceCents int64) string {
	payload := fmt.Sprintf("account_creation:%d", initialBalanceCents)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// GetAccount retrieves a customer account by its ID.
func (s *PostgresService) GetAccount(
	ctx context.Context,
	id int64,
) (*Account, error) {
	account, err := s.accounts.GetByID(
		ctx,
		s.pool,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("get account %d: %w", id, err)
	}

	return account, nil
}

var _ Service = (*PostgresService)(nil)
