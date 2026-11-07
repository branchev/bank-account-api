package banking

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPHandler_Deposit(t *testing.T) {
	serviceCalled := false

	service := &mockAccountService{
		depositFunc: func(
			ctx context.Context,
			request DepositRequest,
		) (DepositResult, error) {
			serviceCalled = true

			if request.AccountID != 123 {
				t.Fatalf(
					"expected account ID 123, got %d",
					request.AccountID,
				)
			}

			if request.AmountCents != 5000 {
				t.Fatalf(
					"expected amount 5000, got %d",
					request.AmountCents,
				)
			}

			if request.IdempotencyKey != "deposit-http-001" {
				t.Fatalf(
					"unexpected idempotency key: %q",
					request.IdempotencyKey,
				)
			}

			return DepositResult{
				StatusCode: http.StatusOK,
				Body: []byte(
					`{"id":123,"currency":"EUR","balance_cents":15000}`,
				),
			}, nil
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
	request.Header.Set("Idempotency-Key", "deposit-http-001")

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP 200, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if !serviceCalled {
		t.Fatal("expected Deposit service method to be called")
	}

	if !strings.Contains(
		recorder.Body.String(),
		`"balance_cents":15000`,
	) {
		t.Fatalf(
			"unexpected response body: %s",
			recorder.Body.String(),
		)
	}

	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("expected application/json, got %q", contentType)
	}
}
