package banking

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	accountpkg "github.com/branchev/bank-account-api/internal/account"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/branchev/bank-account-api/internal/transaction"
	"github.com/jackc/pgx/v5"
)

type mockAccountService struct {
	createAccountFunc func(
		ctx context.Context,
		request CreateAccountRequest,
	) (CreateAccountResult, error)

	getAccountFunc func(
		ctx context.Context,
		id int64,
	) (*accountpkg.Account, error)

	depositFunc func(
		ctx context.Context,
		request DepositRequest,
	) (DepositResult, error)

	transferFunc func(
		ctx context.Context,
		request TransferRequest,
	) (TransferResult, error)
}

func (m *mockAccountService) CreateAccount(
	ctx context.Context,
	request CreateAccountRequest,
) (CreateAccountResult, error) {
	return m.createAccountFunc(ctx, request)
}

func (m *mockAccountService) GetAccount(
	ctx context.Context,
	id int64,
) (*accountpkg.Account, error) {
	if m.getAccountFunc == nil {
		return nil, errors.New("GetAccount is not configured in this mock")
	}

	return m.getAccountFunc(ctx, id)
}

func (m *mockAccountService) Deposit(
	ctx context.Context,
	request DepositRequest,
) (DepositResult, error) {
	if m.depositFunc == nil {
		return DepositResult{}, errors.New("Deposit is not configured in this mock")
	}

	return m.depositFunc(ctx, request)
}

func (m *mockAccountService) Transfer(
	ctx context.Context,
	request TransferRequest,
) (TransferResult, error) {
	if m.transferFunc != nil {
		return m.transferFunc(ctx, request)
	}

	return TransferResult{}, nil
}

// Compile-time check that the mock implements the Service interface.
var _ Service = (*mockAccountService)(nil)

func TestHTTPHandler_CreateAccount(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		idempotencyKey string
		serviceResult  CreateAccountResult
		serviceErr     error
		wantStatus     int
		wantBody       string
		wantServiceHit bool
	}{
		{
			name:           "successful creation",
			body:           `{"initial_balance_cents":10000}`,
			idempotencyKey: "create-account-1",
			serviceResult: CreateAccountResult{
				StatusCode: http.StatusCreated,
				Body:       []byte(`{"id":1,"currency":"EUR","balance_cents":10000}`),
			},
			wantStatus:     http.StatusCreated,
			wantBody:       `"balance_cents":10000`,
			wantServiceHit: true,
		},
		{
			name:       "missing idempotency key",
			body:       `{"initial_balance_cents":10000}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Idempotency-Key header is required",
		},
		{
			name:           "invalid JSON",
			body:           `{"initial_balance_cents":`,
			idempotencyKey: "invalid-json",
			wantStatus:     http.StatusBadRequest,
			wantBody:       "Invalid JSON request body",
		},
		{
			name:           "unknown JSON field",
			body:           `{"initial_balance":10000}`,
			idempotencyKey: "unknown-field",
			wantStatus:     http.StatusBadRequest,
			wantBody:       "Invalid JSON request body",
		},
		{
			name:           "negative initial balance",
			body:           `{"initial_balance_cents":-100}`,
			idempotencyKey: "negative-balance",
			serviceErr:     ErrInvalidInitialBalance,
			wantStatus:     http.StatusBadRequest,
			wantBody:       "initial balance cannot be negative",
			wantServiceHit: true,
		},
		{
			name:           "idempotency conflict",
			body:           `{"initial_balance_cents":10000}`,
			idempotencyKey: "conflicting-key",
			serviceErr:     transaction.ErrIdempotencyConflict,
			wantStatus:     http.StatusConflict,
			wantBody:       "idempotency",
			wantServiceHit: true,
		},
		{
			name:           "unexpected service error",
			body:           `{"initial_balance_cents":10000}`,
			idempotencyKey: "service-error",
			serviceErr:     errors.New("database unavailable"),
			wantStatus:     http.StatusInternalServerError,
			wantBody:       "Internal server error",
			wantServiceHit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serviceCalled := false

			service := &mockAccountService{
				createAccountFunc: func(
					ctx context.Context,
					request CreateAccountRequest,
				) (CreateAccountResult, error) {
					serviceCalled = true

					if request.IdempotencyKey != tt.idempotencyKey {
						t.Fatalf(
							"expected idempotency key %q, got %q",
							tt.idempotencyKey,
							request.IdempotencyKey,
						)
					}

					return tt.serviceResult, tt.serviceErr
				},
			}

			handler := NewHTTPHandler(service)

			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)

			request := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/accounts",
				bytes.NewBufferString(tt.body),
			)

			if tt.idempotencyKey != "" {
				request.Header.Set(
					"Idempotency-Key",
					tt.idempotencyKey,
				)
			}

			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf(
					"expected HTTP %d, got %d: %s",
					tt.wantStatus,
					recorder.Code,
					recorder.Body.String(),
				)
			}

			if !strings.Contains(
				recorder.Body.String(),
				tt.wantBody,
			) {
				t.Fatalf(
					"expected body to contain %q, got %s",
					tt.wantBody,
					recorder.Body.String(),
				)
			}

			if serviceCalled != tt.wantServiceHit {
				t.Fatalf(
					"expected service called=%v, got %v",
					tt.wantServiceHit,
					serviceCalled,
				)
			}

			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
				t.Fatalf(
					"expected application/json, got %q",
					contentType,
				)
			}
		})
	}
}

func TestHTTPHandler_Transfer_Success(t *testing.T) {
	expectedBody := []byte(`{
		"transaction_id": 42,
		"source_account_id": 1,
		"destination_account_id": 2,
		"amount_cents": 3000,
		"destination_balance_cents": 5000
	}`)

	service := &mockAccountService{
		transferFunc: func(
			ctx context.Context,
			request TransferRequest,
		) (TransferResult, error) {
			if request.SourceAccountID != 1 {
				t.Errorf("expected source account ID 1, got %d", request.SourceAccountID)
			}

			if request.DestinationAccountID != 2 {
				t.Errorf("expected destination account ID 2, got %d", request.DestinationAccountID)
			}

			if request.AmountCents != 3000 {
				t.Errorf("expected amount 3000, got %d", request.AmountCents)
			}

			if request.IdempotencyKey != "transfer-test-001" {
				t.Errorf("unexpected idempotency key: %s", request.IdempotencyKey)
			}

			return TransferResult{
				StatusCode: http.StatusOK,
				Body:       expectedBody,
				Replay:     false,
			}, nil
		},
	}

	handler := NewHTTPHandler(service)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{
		"source_account_id": 1,
		"destination_account_id": 2,
		"amount_cents": 3000
	}`

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/transfers",
		strings.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "transfer-test-001")

	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP 200, got %d: %s",
			rec.Code,
			rec.Body.String(),
		)
	}

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected application/json, got %q", got)
	}

	var response struct {
		TransactionID           int64 `json:"transaction_id"`
		SourceAccountID         int64 `json:"source_account_id"`
		DestinationAccountID    int64 `json:"destination_account_id"`
		AmountCents             int64 `json:"amount_cents"`
		DestinationBalanceCents int64 `json:"destination_balance_cents"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if response.TransactionID != 42 ||
		response.SourceAccountID != 1 ||
		response.DestinationAccountID != 2 ||
		response.AmountCents != 3000 ||
		response.DestinationBalanceCents != 5000 {
		t.Fatalf("unexpected transfer response: %+v", response)
	}
}

func TestHTTPHandler_Transfer_Validation(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		idempotencyKey string
	}{
		{
			name: "missing idempotency key",
			body: `{
				"source_account_id": 1,
				"destination_account_id": 2,
				"amount_cents": 3000
			}`,
		},
		{
			name:           "missing source account",
			body:           `{"destination_account_id":2,"amount_cents":3000}`,
			idempotencyKey: "key-1",
		},
		{
			name:           "invalid source account",
			body:           `{"source_account_id":0,"destination_account_id":2,"amount_cents":3000}`,
			idempotencyKey: "key-2",
		},
		{
			name:           "missing destination account",
			body:           `{"source_account_id":1,"amount_cents":3000}`,
			idempotencyKey: "key-3",
		},
		{
			name:           "same source and destination",
			body:           `{"source_account_id":1,"destination_account_id":1,"amount_cents":3000}`,
			idempotencyKey: "key-4",
		},
		{
			name:           "missing amount",
			body:           `{"source_account_id":1,"destination_account_id":2}`,
			idempotencyKey: "key-5",
		},
		{
			name:           "zero amount",
			body:           `{"source_account_id":1,"destination_account_id":2,"amount_cents":0}`,
			idempotencyKey: "key-6",
		},
		{
			name:           "negative amount",
			body:           `{"source_account_id":1,"destination_account_id":2,"amount_cents":-100}`,
			idempotencyKey: "key-7",
		},
		{
			name:           "invalid JSON",
			body:           `{"source_account_id":1,`,
			idempotencyKey: "key-8",
		},
		{
			name:           "unknown field",
			body:           `{"source_account_id":1,"destination_account_id":2,"amount_cents":3000,"currency":"USD"}`,
			idempotencyKey: "key-9",
		},
		{
			name:           "JSON array instead of object",
			body:           `[1,2,3000]`,
			idempotencyKey: "key-10",
		},
		{
			name:           "multiple JSON objects",
			body:           `{"source_account_id":1,"destination_account_id":2,"amount_cents":3000} {}`,
			idempotencyKey: "key-11",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serviceCalled := false

			service := &mockAccountService{
				transferFunc: func(
					ctx context.Context,
					request TransferRequest,
				) (TransferResult, error) {
					serviceCalled = true
					return TransferResult{
						StatusCode: http.StatusOK,
					}, nil
				},
			}

			handler := NewHTTPHandler(service)

			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)

			req := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/transfers",
				strings.NewReader(tt.body),
			)

			req.Header.Set("Content-Type", "application/json")

			if tt.idempotencyKey != "" {
				req.Header.Set("Idempotency-Key", tt.idempotencyKey)
			}

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf(
					"expected HTTP 400, got %d: %s",
					rec.Code,
					rec.Body.String(),
				)
			}

			if serviceCalled {
				t.Fatal("service must not be called for invalid requests")
			}

			var response accountHTTPErrorResponse

			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}

			if response.Error == "" {
				t.Fatal("expected non-empty error message")
			}
		})
	}
}

func TestHTTPHandler_Transfer_ErrorMapping(t *testing.T) {
	tests := []struct {
		name           string
		serviceError   error
		expectedStatus int
	}{
		{
			name:           "account not found",
			serviceError:   pgx.ErrNoRows,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "idempotency conflict",
			serviceError:   transaction.ErrIdempotencyConflict,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "insufficient funds",
			serviceError:   ErrInsufficientFunds,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "balance overflow",
			serviceError:   ErrBalanceOverflow,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "balance underflow",
			serviceError:   ErrBalanceUnderflow,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "unexpected error",
			serviceError:   errors.New("database connection failed"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serviceCalled := false

			service := &mockAccountService{
				transferFunc: func(
					ctx context.Context,
					request TransferRequest,
				) (TransferResult, error) {
					serviceCalled = true

					return TransferResult{}, tt.serviceError
				},
			}

			handler := NewHTTPHandler(service)

			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)

			body := `{
				"source_account_id": 1,
				"destination_account_id": 2,
				"amount_cents": 3000
			}`

			req := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/transfers",
				strings.NewReader(body),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", "transfer-error-test")

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if !serviceCalled {
				t.Fatal("expected service to be called")
			}

			if rec.Code != tt.expectedStatus {
				t.Fatalf(
					"expected HTTP %d, got %d: %s",
					tt.expectedStatus,
					rec.Code,
					rec.Body.String(),
				)
			}

			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("expected application/json, got %q", got)
			}

			var response accountHTTPErrorResponse

			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}

			if response.Error == "" {
				t.Fatal("expected non-empty error message")
			}

			// Internal implementation details must not leak to API clients.
			if tt.expectedStatus == http.StatusInternalServerError &&
				response.Error != "Internal server error" {
				t.Fatalf(
					"expected generic internal error message, got %q",
					response.Error,
				)
			}
		})
	}
}
