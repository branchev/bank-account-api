package account

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPHandler_CreateAccountJSONValidation(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantStatus     int
		wantServiceHit bool
		wantBalance    int64
	}{
		{
			name:           "empty object",
			body:           `{}`,
			wantStatus:     http.StatusCreated,
			wantServiceHit: true,
			wantBalance:    0,
		},
		{
			name:           "explicit zero balance",
			body:           `{"initial_balance_cents":0}`,
			wantStatus:     http.StatusCreated,
			wantServiceHit: true,
			wantBalance:    0,
		},
		{
			name:       "null body",
			body:       `null`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty body",
			body:       ``,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "multiple JSON objects",
			body:       `{} {}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "string instead of integer",
			body:       `{"initial_balance_cents":"100"}`,
			wantStatus: http.StatusBadRequest,
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

					if request.InitialBalanceCents != tt.wantBalance {
						t.Fatalf(
							"expected balance %d, got %d",
							tt.wantBalance,
							request.InitialBalanceCents,
						)
					}

					return CreateAccountResult{
						StatusCode: http.StatusCreated,
						Body:       []byte(`{"id":1,"currency":"EUR","balance_cents":0}`),
					}, nil
				},
			}

			handler := NewHTTPHandler(service)

			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)

			request := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/accounts",
				strings.NewReader(tt.body),
			)
			request.Header.Set("Idempotency-Key", "validation-test")

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
