package banking

import (
	"context"
	"errors"
	accountpkg "github.com/branchev/bank-account-api/internal/account"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestHTTPHandler_GetAccount(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		account        *accountpkg.Account
		serviceErr     error
		wantStatus     int
		wantServiceHit bool
	}{
		{
			name: "existing account",
			path: "/api/v1/accounts/199",
			account: &accountpkg.Account{
				ID:           199,
				Currency:     "EUR",
				BalanceCents: 10000,
			},
			wantStatus:     http.StatusOK,
			wantServiceHit: true,
		},
		{
			name:       "invalid account ID",
			path:       "/api/v1/accounts/abc",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "zero account ID",
			path:       "/api/v1/accounts/0",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:           "nonexistent account",
			path:           "/api/v1/accounts/999999",
			serviceErr:     pgx.ErrNoRows,
			wantStatus:     http.StatusNotFound,
			wantServiceHit: true,
		},
		{
			name:           "database failure",
			path:           "/api/v1/accounts/199",
			serviceErr:     errors.New("database unavailable"),
			wantStatus:     http.StatusInternalServerError,
			wantServiceHit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serviceCalled := false

			service := &mockAccountService{
				getAccountFunc: func(
					ctx context.Context,
					id int64,
				) (*accountpkg.Account, error) {
					serviceCalled = true
					return tt.account, tt.serviceErr
				},
			}

			handler := NewHTTPHandler(service)

			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)

			request := httptest.NewRequest(
				http.MethodGet,
				tt.path,
				nil,
			)

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

			if serviceCalled != tt.wantServiceHit {
				t.Fatalf(
					"expected service called=%v, got %v",
					tt.wantServiceHit,
					serviceCalled,
				)
			}
		})
	}
}
