# Bank Account API

A Go REST API for managing bank accounts and performing financial transactions, developed as a backend engineering take-home assignment.

The implementation focuses on **financial correctness, atomicity, concurrency safety, idempotency, and maintainability**.

## Features

The API supports three core banking operations:

- **Create an account** with a zero or specified initial balance.
- **Deposit funds** into an existing account.
- **Transfer funds** atomically between accounts.

Additional capabilities include:

- Retrieve account details and current balance.
- Idempotent financial operations using the `Idempotency-Key` header.
- Double-entry ledger for financial traceability.
- PostgreSQL transactions and deterministic row-level locking.
- Read-only reconciliation of account balances and ledger transactions.
- OpenAPI documentation and Swagger UI.
- Unit, integration, and concurrency tests.

## Technology Stack

- **Go** — application code and HTTP server.
- **PostgreSQL 17** — transactional persistence.
- **pgx/v5** — PostgreSQL driver and connection pooling.
- **Goose v3** — database migrations.
- **Docker Compose** — local PostgreSQL environment.
- **OpenAPI 3.0.3 / Swagger UI** — API documentation.

The application follows a **modular monolith** architecture. Business logic, persistence, HTTP handlers, and infrastructure concerns are separated into packages within a single deployable application.

## Getting Started

### Prerequisites

- Go (version specified in `go.mod`)
- Docker and Docker Compose
- Goose v3

Install Goose:

```bash
go install github.com/pressly/goose/v3/cmd/goose@v3.29.0
```

Ensure `$(go env GOPATH)/bin` is included in your `PATH`, or invoke Goose using its absolute path.

### 1. Configure the Environment

From the repository root:

```bash
cp .env.example .env
```

The supplied configuration uses PostgreSQL on `127.0.0.1:5433` and the `bank_api` database.

### 2. Start PostgreSQL

```bash
docker compose up -d
docker compose ps
```

### 3. Apply Database Migrations

Load the environment variables:

```bash
set -a
source .env
set +a
```

Apply the migrations:

```bash
goose -dir migrations postgres "$DATABASE_URL" up
```

Check migration status:

```bash
goose -dir migrations postgres "$DATABASE_URL" status
```

### 4. Start the API

```bash
go run ./cmd/api
```

The API requires `DATABASE_URL` and listens on the port configured by `APP_PORT` (default: `8080`). It supports graceful shutdown on SIGINT and SIGTERM.

### 5. Verify the Application

```bash
curl -i http://localhost:8080/health
```

Expected response:

```text
HTTP/1.1 200 OK

OK
```

PostgreSQL connectivity is verified during application startup.

## API Endpoints

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/health` | Application health check |
| `POST` | `/api/v1/accounts` | Create an account |
| `GET` | `/api/v1/accounts/{id}` | Retrieve account details |
| `POST` | `/api/v1/accounts/{id}/deposits` | Deposit funds |
| `POST` | `/api/v1/transfers` | Transfer funds between accounts |
| `GET` | `/api/v1/reconciliation` | Check ledger and account consistency |
| `GET` | `/openapi.yaml` | Bundled OpenAPI specification |
| `GET` | `/swagger` | Interactive Swagger UI |

All monetary values are expressed as **integer EUR cents**.

### Create an Account

```bash
curl -i -X POST http://localhost:8080/api/v1/accounts \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: create-account-001' \
  -d '{"initial_balance_cents":10000}'
```

Example successful response (`201 Created`):

```json
{
  "id": 2,
  "currency": "EUR",
  "balance_cents": 10000
}
```

`initial_balance_cents` is optional and defaults to zero. Negative initial balances are rejected.

### Deposit Funds

```bash
curl -i -X POST http://localhost:8080/api/v1/accounts/2/deposits \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: deposit-001' \
  -d '{"amount_cents":5000}'
```

Replace `2` with an existing customer account ID.

### Transfer Funds

```bash
curl -i -X POST http://localhost:8080/api/v1/transfers \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: transfer-001' \
  -d '{
    "source_account_id": 2,
    "destination_account_id": 3,
    "amount_cents": 2500
  }'
```

Both accounts must exist, and the source account must have sufficient funds.

### Reconciliation

```bash
curl -s http://localhost:8080/api/v1/reconciliation
```

Example response:

```json
{
  "status": "consistent",
  "accounts_checked": 8,
  "transactions_checked": 13,
  "discrepancies": []
}
```

Reconciliation checks that:

- Each account balance matches the sum of its ledger entries.
- Ledger entries for each financial transaction sum to zero.

It executes inside a read-only PostgreSQL transaction using `REPEATABLE READ` isolation, providing a consistent database snapshot.

A successful reconciliation request returns HTTP `200`, even if discrepancies are detected. The `status` field indicates whether the data is consistent.

## Financial Integrity

### Atomicity

Financial operations execute inside PostgreSQL transactions.

Account balances, transaction records, ledger entries, and stored idempotency responses are committed together.

If an operation fails, its database changes are rolled back.

### Double-Entry Ledger

Financial movements are recorded as balanced ledger entries.

For example, funding a customer account with EUR 100 creates:

| Account | Amount |
|---|---:|
| Internal system account | -10000 cents |
| Customer account | +10000 cents |
| **Total** | **0 cents** |

The fundamental ledger invariant is:

```text
SUM(ledger_entries.amount_cents) = 0
```

for each financial transaction.

Existing ledger entries are protected against modification by database triggers.

### Concurrency Safety

Account rows involved in financial operations are locked using PostgreSQL row-level locks.

When multiple accounts are involved, locks are acquired in ascending account-ID order to reduce deadlock risk.

This prevents concurrent operations from independently spending the same available balance.

### Idempotency

Financial write requests use the `Idempotency-Key` header.

- Repeating an operation with the same key and payload returns its stored result without executing the financial movement again.
- Reusing the same key with a different request produces `409 Conflict`.
- Idempotency records and financial changes are persisted in the same database transaction.

This protects against duplicate execution caused by client retries or network failures.

### Money Representation

Money is represented using signed `int64` cents in Go and `BIGINT` in PostgreSQL.

Floating-point arithmetic is not used for financial amounts.

Customer account balances cannot become negative. The internal system funding account may have a negative balance.

## Testing

The project uses Go's standard `testing` package.

Tests cover API validation, persistence, financial operations, idempotency, concurrency, and integration with PostgreSQL.

### Test Database

Integration tests use a dedicated `bank_api_test` database.

Start PostgreSQL:

```bash
docker compose up -d
```

Create the test database if it does not already exist:

```bash
docker compose exec -T postgres \
  psql -U bank -d bank_api \
  -c 'CREATE DATABASE bank_api_test;'
```

Configure the test connection:

```bash
export TEST_DATABASE_URL='postgres://bank:bank_dev_password@127.0.0.1:5433/bank_api_test?sslmode=disable'
```

For the full test suite, point `DATABASE_URL` to the same dedicated test database because some tests use that variable:

```bash
export DATABASE_URL="$TEST_DATABASE_URL"
```

**Never point test environment variables at a production database.**

Integration tests use isolated PostgreSQL schemas to avoid interfering with application data.

### Run Tests

```bash
go test ./... -count=1
```

With the race detector:

```bash
go test -race ./... -count=1
```

Static analysis:

```bash
go vet ./...
```

## API Documentation

Interactive Swagger UI:

```text
http://localhost:8080/swagger
```

Bundled OpenAPI specification:

```text
http://localhost:8080/openapi.yaml
```

The OpenAPI source is organized into modular files:

```text
api/
├── openapi.yaml
├── openapi.bundle.yaml
├── paths/
├── schemas/
└── swagger/
```

The generated bundle is committed to the repository and embedded into the Go application.

To regenerate it after changing the source specification:

```bash
docker run --rm \
  -v "$(pwd):/work" \
  -w /work \
  redocly/cli:latest \
  bundle api/openapi.yaml \
  --output api/openapi.bundle.yaml
```

Swagger UI loads frontend assets from a CDN and requires internet access to render.

The OpenAPI specification uses a relative server URL (`/`), allowing Swagger UI to send requests to the same host and port serving the application.

**Important:** `api/openapi.bundle.yaml` is generated from the modular OpenAPI source files and embedded into the Go binary at build time.

After modifying the OpenAPI specification:

1. Regenerate `api/openapi.bundle.yaml` using the command above.
2. Restart the Go application (`go run ./cmd/api`).
3. Refresh Swagger UI in the browser.

Always commit the generated bundle together with the corresponding OpenAPI source changes. Regenerating the bundle is only necessary when the OpenAPI specification changes.

## Architecture

The application separates HTTP transport, banking services, persistence, and infrastructure concerns.

The main packages include:

| Package | Responsibility |
|---|---|
| `cmd/api` | Application entry point and dependency wiring |
| `api` | OpenAPI specification and Swagger UI |
| `internal/banking` | Banking business operations and HTTP handlers |
| `internal/account` | Account model and persistence |
| `internal/ledger` | Double-entry ledger persistence |
| `internal/transaction` | Financial transaction and idempotency persistence |
| `internal/reconciliation` | Read-only financial consistency checks |
| `internal/platform` | Database and HTTP infrastructure |
| `migrations` | PostgreSQL schema migrations |

Business logic is coordinated by services, while repositories encapsulate database operations.

## Design Decisions and Trade-offs

**PostgreSQL transactions over distributed coordination.** The application uses a single transactional database to provide strong consistency without introducing message brokers, distributed transactions, or additional infrastructure.

**Modular monolith over microservices.** The scope does not justify independently deployed services. A modular monolith keeps the implementation easier to run, test, and reason about.

**Integer cents over floating-point amounts.** This avoids floating-point precision problems in financial calculations.

**Persisted account balances with a ledger.** Account balances provide efficient reads, while immutable ledger entries provide an auditable record of financial movements. Reconciliation can detect inconsistencies between the two.

**Synchronous operations.** Financial writes are completed within the HTTP request. Background processing and asynchronous workflows are intentionally outside the scope.

**Reconciliation is diagnostic.** The reconciliation endpoint detects discrepancies but does not automatically modify account balances or repair ledger entries.

### Possible Future Improvements

- Authentication and authorization.
- Pagination and filtering for transaction history.
- Metrics and observability.
- Additional reconciliation coverage and operational tooling.

These improvements are intentionally deferred to keep the take-home implementation focused on the requested banking operations and their correctness.

## Development Approach

The implementation prioritizes correctness and clarity over unnecessary complexity.

The core approach is to define financial invariants, implement transactional business operations, verify behavior through tests, and document architectural decisions and trade-offs.