package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/branchev/bank-account-api/api"
	"github.com/branchev/bank-account-api/internal/account"
	"github.com/branchev/bank-account-api/internal/banking"
	"github.com/branchev/bank-account-api/internal/ledger"
	"github.com/branchev/bank-account-api/internal/platform/database"
	"github.com/branchev/bank-account-api/internal/platform/httpmiddleware"
	"github.com/branchev/bank-account-api/internal/reconciliation"
	"github.com/branchev/bank-account-api/internal/transaction"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	// Initialize database connection.
	pool, err := database.NewPool(ctx, databaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()

	log.Println("Connected to PostgreSQL")

	// Initialize repositories.
	accountRepository := account.NewPostgresRepository()
	ledgerRepository := ledger.NewPostgresRepository()
	transactionRepository := transaction.NewPostgresRepository()

	// Initialize services.
	accountService := banking.NewPostgresService(
		pool,
		accountRepository,
		ledgerRepository,
		transactionRepository,
	)

	reconciliationService := reconciliation.NewPostgresService(pool)

	// Initialize HTTP handlers.
	accountHandler := banking.NewHTTPHandler(accountService)
	reconciliationHandler := reconciliation.NewHTTPHandler(reconciliationService)

	// Register HTTP routes.
	mux := http.NewServeMux()

	// Swagger UI and OpenAPI documentation.
	api.RegisterRoutes(mux)

	// Banking endpoints.
	accountHandler.RegisterRoutes(mux)

	// Health check.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Read-only reconciliation endpoint.
	mux.HandleFunc(
		"GET /api/v1/reconciliation",
		reconciliationHandler.Reconcile,
	)

	// Configure HTTP server.
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           httpmiddleware.RequestLogger(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Start HTTP server.
	go func() {
		log.Printf("Bank API listening on port %s", port)

		if err := server.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server error: %v", err)
			stop()
		}
	}()

	// Wait for termination signal.
	<-ctx.Done()

	// Graceful shutdown.
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown failed: %v", err)
	}

	log.Println("Bank API stopped")
}
