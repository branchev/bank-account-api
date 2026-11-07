package account

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/branchev/bank-account-api/internal/transaction"
)

type mockAccountService struct {
	createAccountFunc func(
		ctx context.Context,
		request CreateAccountRequest,
	) (CreateAccountResult, error)

	getAccountFunc func(
		ctx context.Context,
		id int64,
	) (*Account, error)
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
) (*Account, error) {
	if m.getAccountFunc == nil {
		return nil, errors.New("GetAccount is not configured in this mock")
	}

	return m.getAccountFunc(ctx, id)
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
