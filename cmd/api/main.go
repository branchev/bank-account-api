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

	// Initialize the account service.
	accountService := banking.NewPostgresService(
		pool,
		accountRepository,
		ledgerRepository,
		transactionRepository,
	)

	// Initialize the HTTP handler.
	accountHandler := banking.NewHTTPHandler(accountService)

	// Register HTTP routes.
	mux := http.NewServeMux()

	api.RegisterRoutes(mux)
	accountHandler.RegisterRoutes(mux)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("Bank API listening on port %s", port)

		if err := server.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server error: %v", err)
			stop()
		}
	}()

	<-ctx.Done()

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
