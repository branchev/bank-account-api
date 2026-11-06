package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocumentationRoutes(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux)

	tests := []struct {
		name        string
		path        string
		contentType string
		contains    string
	}{
		{
			name:        "OpenAPI specification",
			path:        "/openapi.yaml",
			contentType: "application/yaml",
			contains:    "openapi: 3.0.3",
		},
		{
			name:        "Swagger UI",
			path:        "/swagger",
			contentType: "text/html",
			contains:    "SwaggerUIBundle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", rec.Code)
			}

			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, tt.contentType) {
				t.Errorf("expected Content-Type %q, got %q", tt.contentType, got)
			}

			if !strings.Contains(rec.Body.String(), tt.contains) {
				t.Errorf("response body does not contain %q", tt.contains)
			}
		})
	}
}
