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
	"github.com/jackc/pgx/v5"
)

func TestHTTPHandler_DepositErrors(t *testing.T) {
	tests := []struct {
		name       string
		serviceErr error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "account not found",
			serviceErr: pgx.ErrNoRows,
			wantStatus: http.StatusNotFound,
			wantBody:   "Account not found",
		},
		{
			name:       "idempotency conflict",
			serviceErr: transaction.ErrIdempotencyConflict,
			wantStatus: http.StatusConflict,
			wantBody:   "idempotency",
		},
		{
			name:       "balance overflow",
			serviceErr: ErrBalanceOverflow,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   "account balance overflow",
		},
		{
			name:       "balance underflow",
			serviceErr: ErrBalanceUnderflow,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   "account balance underflow",
		},
		{
			name:       "unexpected database error",
			serviceErr: errors.New("database unavailable"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serviceCalled := false

			service := &mockAccountService{
				depositFunc: func(
					ctx context.Context,
					request DepositRequest,
				) (DepositResult, error) {
					serviceCalled = true

					return DepositResult{}, tt.serviceErr
				},
			}

			handler := NewHTTPHandler(service)

			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)

			request := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/accounts/123/deposits",
				bytes.NewBufferString(`{"amount_cents":5000}`),
			)

			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "deposit-error-test")

			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, request)

			if !serviceCalled {
				t.Fatal("expected Deposit service method to be called")
			}

			if recorder.Code != tt.wantStatus {
				t.Fatalf(
					"expected HTTP %d, got %d: %s",
					tt.wantStatus,
					recorder.Code,
					recorder.Body.String(),
				)
			}

			if !strings.Contains(recorder.Body.String(), tt.wantBody) {
				t.Fatalf(
					"expected body to contain %q, got %s",
					tt.wantBody,
					recorder.Body.String(),
				)
			}

			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("expected application/json, got %q", got)
			}
		})
	}
}
