package account

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/branchev/bank-account-api/internal/transaction"
	"github.com/jackc/pgx/v5"
)

const maxRequestBodyBytes = 1 << 20 // 1 MiB

// HTTPHandler handles HTTP requests related to bank accounts.
type HTTPHandler struct {
	service Service
}

// NewHTTPHandler creates an account HTTP handler.
func NewHTTPHandler(service Service) *HTTPHandler {
	return &HTTPHandler{
		service: service,
	}
}

// RegisterRoutes registers account-related HTTP endpoints.
func (h *HTTPHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc(
		"POST /api/v1/accounts",
		h.CreateAccount,
	)

	mux.HandleFunc(
		"GET /api/v1/accounts/{id}",
		h.GetAccount,
	)

	mux.HandleFunc(
		"POST /api/v1/accounts/{id}/deposits",
		h.Deposit,
	)

	mux.HandleFunc(
		"POST /api/v1/transfers",
		h.Transfer,
	)
}

type accountHTTPErrorResponse struct {
	Error string `json:"error"`
}

// CreateAccount handles POST /api/v1/accounts.
func (h *HTTPHandler) CreateAccount(
	w http.ResponseWriter,
	r *http.Request,
) {
	idempotencyKey := strings.TrimSpace(
		r.Header.Get("Idempotency-Key"),
	)

	if idempotencyKey == "" {
		writeAccountError(
			w,
			http.StatusBadRequest,
			"Idempotency-Key header is required",
		)
		return
	}

	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		maxRequestBodyBytes,
	)

	var raw json.RawMessage

	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(&raw); err != nil {
		writeAccountError(
			w,
			http.StatusBadRequest,
			"Invalid JSON request body",
		)
		return
	}

	// Only JSON objects are accepted.
	if len(raw) == 0 || raw[0] != '{' {
		writeAccountError(
			w,
			http.StatusBadRequest,
			"Request body must be a JSON object",
		)
		return
	}

	// Reject multiple JSON values.
	var extra any

	if err := decoder.Decode(&extra); err != io.EOF {
		writeAccountError(
			w,
			http.StatusBadRequest,
			"Request body must contain exactly one JSON object",
		)
		return
	}

	var payload struct {
		InitialBalanceCents int64 `json:"initial_balance_cents"`
	}

	payloadDecoder := json.NewDecoder(bytes.NewReader(raw))
	payloadDecoder.DisallowUnknownFields()

	if err := payloadDecoder.Decode(&payload); err != nil {
		writeAccountError(
			w,
			http.StatusBadRequest,
			"Invalid JSON request body",
		)
		return
	}

	result, err := h.service.CreateAccount(
		r.Context(),
		CreateAccountRequest{
			InitialBalanceCents: payload.InitialBalanceCents,
			IdempotencyKey:      idempotencyKey,
		},
	)

	if err != nil {
		switch {
		case errors.Is(err, ErrMissingIdempotencyKey),
			errors.Is(err, ErrInvalidInitialBalance):
			writeAccountError(
				w,
				http.StatusBadRequest,
				err.Error(),
			)

		case errors.Is(err, transaction.ErrIdempotencyConflict):
			writeAccountError(
				w,
				http.StatusConflict,
				err.Error(),
			)

		default:
			writeAccountError(
				w,
				http.StatusInternalServerError,
				"Internal server error",
			)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(result.StatusCode)

	_, _ = w.Write(result.Body)
}

// GetAccount handles GET /api/v1/accounts/{id}.
func (h *HTTPHandler) GetAccount(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Extract the path parameter using Go's ServeMux.
	rawID := r.PathValue("id")

	accountID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || accountID <= 0 {
		writeAccountError(
			w,
			http.StatusBadRequest,
			"Invalid account ID",
		)
		return
	}

	account, err := h.service.GetAccount(
		r.Context(),
		accountID,
	)

	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			writeAccountError(
				w,
				http.StatusNotFound,
				"Account not found",
			)

		default:
			writeAccountError(
				w,
				http.StatusInternalServerError,
				"Internal server error",
			)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(account)
}

// Deposit handles POST /api/v1/accounts/{id}/deposits.
func (h *HTTPHandler) Deposit(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Validate account ID.
	accountID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || accountID <= 0 {
		writeAccountError(w, http.StatusBadRequest, "Invalid account ID")
		return
	}

	// Validate idempotency key.
	idempotencyKey := strings.TrimSpace(
		r.Header.Get("Idempotency-Key"),
	)
	if idempotencyKey == "" {
		writeAccountError(
			w,
			http.StatusBadRequest,
			"Idempotency-Key header is required",
		)
		return
	}

	// Limit request body size.
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	var raw json.RawMessage
	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(&raw); err != nil {
		writeAccountError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}

	if len(raw) == 0 || raw[0] != '{' {
		writeAccountError(w, http.StatusBadRequest, "Request body must be a JSON object")
		return
	}

	// Reject multiple JSON values.
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeAccountError(
			w,
			http.StatusBadRequest,
			"Request body must contain exactly one JSON object",
		)
		return
	}

	// Use a pointer to distinguish a missing amount from an explicit zero.
	var payload struct {
		AmountCents *int64 `json:"amount_cents"`
	}

	payloadDecoder := json.NewDecoder(bytes.NewReader(raw))
	payloadDecoder.DisallowUnknownFields()

	if err := payloadDecoder.Decode(&payload); err != nil {
		writeAccountError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}

	if payload.AmountCents == nil || *payload.AmountCents <= 0 {
		writeAccountError(
			w,
			http.StatusBadRequest,
			"deposit amount must be positive",
		)
		return
	}

	result, err := h.service.Deposit(
		r.Context(),
		DepositRequest{
			AccountID:      accountID,
			AmountCents:    *payload.AmountCents,
			IdempotencyKey: idempotencyKey,
		},
	)

	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidDepositAmount),
			errors.Is(err, ErrMissingIdempotencyKey):
			writeAccountError(w, http.StatusBadRequest, err.Error())

		case errors.Is(err, pgx.ErrNoRows):
			writeAccountError(w, http.StatusNotFound, "Account not found")

		case errors.Is(err, transaction.ErrIdempotencyConflict):
			writeAccountError(w, http.StatusConflict, err.Error())

		case errors.Is(err, ErrBalanceOverflow),
			errors.Is(err, ErrBalanceUnderflow):
			writeAccountError(w, http.StatusUnprocessableEntity, err.Error())

		default:
			writeAccountError(
				w,
				http.StatusInternalServerError,
				"Internal server error",
			)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(result.StatusCode)

	_, _ = w.Write(result.Body)
}

// Transfer handles POST /api/v1/transfers.
func (h *HTTPHandler) Transfer(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Validate idempotency key.
	idempotencyKey := strings.TrimSpace(
		r.Header.Get("Idempotency-Key"),
	)
	if idempotencyKey == "" {
		writeAccountError(
			w,
			http.StatusBadRequest,
			"Idempotency-Key header is required",
		)
		return
	}

	// Limit request body size.
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	var raw json.RawMessage
	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(&raw); err != nil {
		writeAccountError(
			w,
			http.StatusBadRequest,
			"Invalid JSON request body",
		)
		return
	}

	// Only JSON objects are accepted.
	if len(raw) == 0 || raw[0] != '{' {
		writeAccountError(
			w,
			http.StatusBadRequest,
			"Request body must be a JSON object",
		)
		return
	}

	// Reject multiple JSON values.
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeAccountError(
			w,
			http.StatusBadRequest,
			"Request body must contain exactly one JSON object",
		)
		return
	}

	// Pointers distinguish missing fields from explicit zero values.
	var payload struct {
		SourceAccountID      *int64 `json:"source_account_id"`
		DestinationAccountID *int64 `json:"destination_account_id"`
		AmountCents          *int64 `json:"amount_cents"`
	}

	payloadDecoder := json.NewDecoder(bytes.NewReader(raw))
	payloadDecoder.DisallowUnknownFields()

	if err := payloadDecoder.Decode(&payload); err != nil {
		writeAccountError(
			w,
			http.StatusBadRequest,
			"Invalid JSON request body",
		)
		return
	}

	if payload.SourceAccountID == nil || *payload.SourceAccountID <= 0 {
		writeAccountError(
			w,
			http.StatusBadRequest,
			"Invalid source account ID",
		)
		return
	}

	if payload.DestinationAccountID == nil || *payload.DestinationAccountID <= 0 {
		writeAccountError(
			w,
			http.StatusBadRequest,
			"Invalid destination account ID",
		)
		return
	}

	if *payload.SourceAccountID == *payload.DestinationAccountID {
		writeAccountError(
			w,
			http.StatusBadRequest,
			ErrSameAccount.Error(),
		)
		return
	}

	if payload.AmountCents == nil || *payload.AmountCents <= 0 {
		writeAccountError(
			w,
			http.StatusBadRequest,
			ErrInvalidTransferAmount.Error(),
		)
		return
	}

	result, err := h.service.Transfer(
		r.Context(),
		TransferRequest{
			SourceAccountID:      *payload.SourceAccountID,
			DestinationAccountID: *payload.DestinationAccountID,
			AmountCents:          *payload.AmountCents,
			IdempotencyKey:       idempotencyKey,
		},
	)

	if err != nil {
		switch {
		case errors.Is(err, ErrMissingIdempotencyKey),
			errors.Is(err, ErrInvalidTransferAmount),
			errors.Is(err, ErrSameAccount):
			writeAccountError(w, http.StatusBadRequest, err.Error())

		case errors.Is(err, pgx.ErrNoRows):
			writeAccountError(w, http.StatusNotFound, "Account not found")

		case errors.Is(err, transaction.ErrIdempotencyConflict):
			writeAccountError(w, http.StatusConflict, err.Error())

		case errors.Is(err, ErrInsufficientFunds),
			errors.Is(err, ErrBalanceOverflow),
			errors.Is(err, ErrBalanceUnderflow):
			writeAccountError(w, http.StatusUnprocessableEntity, err.Error())

		default:
			writeAccountError(
				w,
				http.StatusInternalServerError,
				"Internal server error",
			)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(result.StatusCode)

	_, _ = w.Write(result.Body)
}

// writeAccountError writes a consistent JSON error response.
func writeAccountError(
	w http.ResponseWriter,
	statusCode int,
	message string,
) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	_ = json.NewEncoder(w).Encode(
		accountHTTPErrorResponse{
			Error: message,
		},
	)
}
