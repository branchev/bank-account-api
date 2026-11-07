package httpmiddleware

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLogger(t *testing.T) {
	var logs strings.Builder

	logger := log.Default()
	originalOutput := logger.Writer()
	logger.SetOutput(&logs)
	t.Cleanup(func() {
		logger.SetOutput(originalOutput)
	})

	handler := RequestLogger(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		},
	))

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/accounts",
		nil,
	)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf(
			"expected status 201, got %d",
			recorder.Code,
		)
	}

	logOutput := logs.String()

	for _, expected := range []string{
		"POST",
		"/api/v1/accounts",
		"201",
	} {
		if !strings.Contains(logOutput, expected) {
			t.Errorf(
				"expected log to contain %q, got %q",
				expected,
				logOutput,
			)
		}
	}

	_ = io.Discard
}
