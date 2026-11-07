package reconciliation

import (
	"encoding/json"
	"log"
	"net/http"
)

type HTTPHandler struct {
	service Service
}

func NewHTTPHandler(service Service) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func (h *HTTPHandler) Reconcile(w http.ResponseWriter, r *http.Request) {
	report, err := h.service.Reconcile(r.Context())
	if err != nil {
		log.Printf("reconciliation failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "internal server error",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(report); err != nil {
		log.Printf("encode reconciliation report: %v", err)
	}
}
