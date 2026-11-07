package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// NewPostgresPool creates an isolated PostgreSQL schema for each test,
// applies all migrations, and registers automatic cleanup.
func NewPostgresPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required")
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}

	// Never run integration tests against the production database.
	if config.ConnConfig.Database != "bank_api_test" {
		t.Fatalf(
			"refusing to run integration tests against database %q",
			config.ConnConfig.Database,
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	// Create a unique schema for this test.
	schema := fmt.Sprintf("test_%d", time.Now().UnixNano())

	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create admin pool: %v", err)
	}

	_, err = adminPool.Exec(ctx, "CREATE SCHEMA "+schema)
	if err != nil {
		adminPool.Close()
		t.Fatalf("create test schema: %v", err)
	}

	// Cleanup callbacks execute in reverse registration order.
	// The test pool must close before dropping the schema.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()

		_, err := adminPool.Exec(
			cleanupCtx,
			"DROP SCHEMA "+schema+" CASCADE",
		)
		if err != nil {
			t.Errorf("drop test schema %s: %v", schema, err)
		}

		adminPool.Close()
	})

	// Configure the application pool to use the isolated schema.
	config.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("create isolated pool: %v", err)
	}

	t.Cleanup(pool.Close)

	// Configure a separate database connection for Goose.
	migrationConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse migration database URL: %v", err)
	}

	migrationConfig.ConnConfig.RuntimeParams["search_path"] = schema

	// OpenDB preserves the pgx connection configuration,
	// including the search_path runtime parameter.
	migrationDB := stdlib.OpenDB(*migrationConfig.ConnConfig)
	defer migrationDB.Close()

	// Verify that migrations will run in the correct schema.
	var currentSchema string
	var searchPath string

	err = migrationDB.QueryRowContext(
		ctx,
		"SELECT current_schema(), current_setting('search_path')",
	).Scan(&currentSchema, &searchPath)

	if err != nil {
		t.Fatalf("inspect migration connection: %v", err)
	}

	t.Logf("migration current_schema: %s", currentSchema)
	t.Logf("migration search_path: %s", searchPath)

	if currentSchema != schema {
		t.Fatalf(
			"migration connection uses schema %q, expected %q",
			currentSchema,
			schema,
		)
	}

	// Locate the project's existing SQL migrations.
	migrationsDir := findMigrationsDir(t)

	// Use a dedicated Goose provider instead of global settings.
	// This allows tests to run independently.
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		migrationDB,
		os.DirFS(migrationsDir),
	)
	if err != nil {
		t.Fatalf("create goose provider: %v", err)
	}

	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("apply test migrations: %v", err)
	}

	return pool
}

// findMigrationsDir locates the migrations directory
// relative to this source file.
func findMigrationsDir(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine helper file location")
	}

	root := filepath.Clean(
		filepath.Join(
			filepath.Dir(currentFile),
			"../../../..",
		),
	)

	dir := filepath.Join(root, "migrations")

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("migrations directory not found: %v", err)
	}

	return dir
}
