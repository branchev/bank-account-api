package banking

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPHandler_DepositValidation(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		body           string
		idempotencyKey string
		wantStatus     int
		wantBody       string
	}{
		{
			name:       "missing idempotency key",
			path:       "/api/v1/accounts/123/deposits",
			body:       `{"amount_cents":5000}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Idempotency-Key header is required",
		},
		{
			name:           "invalid account ID",
			path:           "/api/v1/accounts/abc/deposits",
			body:           `{"amount_cents":5000}`,
			idempotencyKey: "deposit-invalid-id",
			wantStatus:     http.StatusBadRequest,
			wantBody:       "Invalid account ID",
		},
		{
			name:           "zero account ID",
			path:           "/api/v1/accounts/0/deposits",
			body:           `{"amount_cents":5000}`,
			idempotencyKey: "deposit-zero-id",
			wantStatus:     http.StatusBadRequest,
			wantBody:       "Invalid account ID",
		},
		{
			name:           "missing amount",
			path:           "/api/v1/accounts/123/deposits",
			body:           `{}`,
			idempotencyKey: "deposit-missing-amount",
			wantStatus:     http.StatusBadRequest,
			wantBody:       "deposit amount must be positive",
		},
		{
			name:           "zero amount",
			path:           "/api/v1/accounts/123/deposits",
			body:           `{"amount_cents":0}`,
			idempotencyKey: "deposit-zero-amount",
			wantStatus:     http.StatusBadRequest,
			wantBody:       "deposit amount must be positive",
		},
		{
			name:           "negative amount",
			path:           "/api/v1/accounts/123/deposits",
			body:           `{"amount_cents":-100}`,
			idempotencyKey: "deposit-negative",
			wantStatus:     http.StatusBadRequest,
			wantBody:       "deposit amount must be positive",
		},
		{
			name:           "invalid JSON",
			path:           "/api/v1/accounts/123/deposits",
			body:           `{"amount_cents":`,
			idempotencyKey: "deposit-invalid-json",
			wantStatus:     http.StatusBadRequest,
			wantBody:       "Invalid JSON request body",
		},
		{
			name:           "unknown JSON field",
			path:           "/api/v1/accounts/123/deposits",
			body:           `{"amount_cents":5000,"unexpected":true}`,
			idempotencyKey: "deposit-unknown-field",
			wantStatus:     http.StatusBadRequest,
			wantBody:       "Invalid JSON request body",
		},
		{
			name:           "multiple JSON objects",
			path:           "/api/v1/accounts/123/deposits",
			body:           `{"amount_cents":5000}{"amount_cents":7000}`,
			idempotencyKey: "deposit-multiple-json",
			wantStatus:     http.StatusBadRequest,
			wantBody:       "Request body must contain exactly one JSON object",
		},
		{
			name:           "JSON array instead of object",
			path:           "/api/v1/accounts/123/deposits",
			body:           `[{"amount_cents":5000}]`,
			idempotencyKey: "deposit-array",
			wantStatus:     http.StatusBadRequest,
			wantBody:       "Request body must be a JSON object",
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
					return DepositResult{
						StatusCode: http.StatusOK,
					}, nil
				},
			}

			handler := NewHTTPHandler(service)
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)

			req := httptest.NewRequest(
				http.MethodPost,
				tt.path,
				bytes.NewBufferString(tt.body),
			)

			if tt.idempotencyKey != "" {
				req.Header.Set("Idempotency-Key", tt.idempotencyKey)
			}

			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)

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
					"expected response to contain %q, got %s",
					tt.wantBody,
					recorder.Body.String(),
				)
			}

			if serviceCalled {
				t.Fatal("Deposit service must not be called for invalid requests")
			}

			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("expected application/json, got %q", got)
			}
		})
	}
}
