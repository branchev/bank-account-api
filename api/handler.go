package api

import (
	"embed"
	"net/http"
)

//go:embed openapi.yaml swagger/index.html
var documentation embed.FS

func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /openapi.yaml", serveOpenAPI)
	mux.HandleFunc("GET /swagger", serveSwaggerUI)
}

func serveOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	http.ServeFileFS(w, r, documentation, "openapi.yaml")
}

func serveSwaggerUI(w http.ResponseWriter, r *http.Request) {
	content, err := documentation.ReadFile("swagger/index.html")
	if err != nil {
		http.Error(
			w,
			"Swagger UI unavailable",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write(content)
}
