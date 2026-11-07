# Bank Account API

A Go-based banking REST API developed as a backend engineering take-home assignment.

## Overview

The application is designed around three core financial operations:

- Create a bank account with a zero or specified initial balance.
- Deposit money into an existing account.
- Transfer money atomically between accounts.

The implementation prioritizes **financial correctness, concurrency safety, maintainability, and testability**. Development is incremental: the account-creation workflow is implemented and tested; deposits and transfers are upcoming work.

## Architecture

The project follows a **modular monolith** architecture, with PostgreSQL as its transactional data store. Business domains are separated within one deployable application, avoiding unnecessary distributed-system complexity while preserving clear module boundaries.

### Design Goals

- ACID transactions and concurrency-safe balance changes.
- Double-entry ledger for financial traceability.
- Idempotent financial write operations.
- Consistent account balances.
- Internal reconciliation (planned).
- Reproducible local development with Docker Compose.
- Versioned REST API and modular OpenAPI documentation.

### Technology Stack

- **Go** — application code, HTTP server, routing, and tests.
- **PostgreSQL 17** — transactional data store.
- **pgx/v5** — PostgreSQL driver and connection pooling.
- **Goose v3** — schema migrations.
- **Docker Compose** — local PostgreSQL environment.
- **OpenAPI 3.0.3** — API contract.
- **Swagger UI** — interactive API documentation.
- **vacuum** — bundling modular OpenAPI YAML files.

## Project Status

**Work in progress.**

### Implemented

- HTTP server with graceful shutdown and PostgreSQL connectivity checks at startup.
- PostgreSQL Docker Compose setup and Goose migrations.
- Accounts, transactions, and immutable ledger-entry persistence.
- Seeded internal system funding account and database constraints.
- `GET /health`, `GET /openapi.yaml`, and `GET /swagger`.
- `POST /api/v1/accounts`, including optional initial funding.
- Atomic account creation and initial funding in a single PostgreSQL transaction.
- Double-entry posting for positive initial balances.
- Idempotency-key reservation, response replay, and conflict detection.
- Deterministic account row-lock ordering and signed integer overflow checks for initial funding.
- Repository, service, HTTP, integration, and concurrency tests, including race-detector runs.
- Isolated PostgreSQL schemas for integration tests.

### In Progress / Planned

- `GET /api/v1/accounts/{id}` — repository lookup and service method exist; HTTP endpoint is not yet registered.
- `POST /api/v1/accounts/{id}/deposits`.
- `POST /api/v1/transfers`.
- Ledger/account balance reconciliation command.
- OpenAPI coverage for business endpoints and regeneration of the embedded bundle.
- Additional end-to-end and financial-invariant tests as features are completed.

## Architecture and Conventions

### API Conventions

| Area | Convention |
|---|---|
| API versioning | `/api/v1` |
| Resource naming | RESTful plural nouns (`/accounts`, `/transfers`) |
| Request/response format | JSON with `snake_case` field names |
| Error format | JSON object containing an `error` field |
| API specification | OpenAPI 3.0.3 |
| Specification organization | Modular YAML with external `$ref` references |
| Interactive documentation | `/swagger` |
| Bundled specification | `/openapi.yaml` |
| Health check | `/health` (outside the versioned namespace) |
| Idempotency | `Idempotency-Key` header for financial writes |

### Go Conventions

- Follow idiomatic Go naming (`MixedCaps` for identifiers).
- Keep HTTP handlers responsible for transport concerns: parsing, validation, and responses.
- Put business rules and transactional coordination in domain services.
- Encapsulate persistence behind repository interfaces where useful.
- Propagate `context.Context` through request-scoped operations.
- Prefer the standard library unless a dependency adds clear value.
- Keep `cmd/api/main.go` focused on dependency wiring and server lifecycle.

### Persistence and Financial Integrity

- **Currency:** EUR.
- **Money representation:** signed `int64` cents in Go and `BIGINT` in PostgreSQL; no floating-point arithmetic for money.
- **Account balances:** customer balances cannot be negative; the internal system funding account may carry a negative balance.
- **Atomicity:** account creation, optional initial funding, ledger entries, and stored idempotency response are committed together.
- **Double-entry ledger:** each positive initial funding operation debits the system account and credits the customer account by equal amounts.
- **Immutability:** database triggers prevent modifications to existing ledger entries.
- **Concurrency:** affected account rows are locked in ascending account-ID order to reduce deadlock risk.
- **Idempotency:** a repeated key with the same operation and payload returns the stored result; reusing the key with a different payload returns `409 Conflict`.
- **Reconciliation:** independently checking account balances against ledger totals is planned.

For a funded account, the ledger invariant is:

```text
SUM(ledger_entries.amount_cents) = 0  (per financial transaction)
```

For example, funding a customer account with EUR 100.00 creates entries of `-10000` cents for the system account and `+10000` cents for the customer account.

### Project Structure

```text
bank-api/
├── api/
│   ├── handler.go
│   ├── handler_test.go
│   ├── openapi.yaml
│   ├── openapi.bundle.yaml
│   ├── paths/
│   ├── schemas/
│   └── swagger/
├── cmd/
│   ├── api/
│   │   └── main.go
│   └── reconcile/                  # Planned
├── internal/
│   ├── account/                    # Model, repository, service, HTTP handler, tests
│   ├── ledger/                     # Balanced ledger posting
│   ├── transaction/                # Idempotency and transaction persistence
│   ├── transfer/                   # Planned
│   └── platform/
│       └── database/               # Pool, query interface, test utilities
├── migrations/
│   ├── 20261106105043_init_schema.sql
│   └── 20261106124712_unique_system_account.sql
├── docker-compose.yml
├── go.mod
├── go.sum
└── README.md
```

The top-level `api` package serves embedded API documentation. Domain packages under `internal` own business behavior and persistence. The `cmd/api` package is the application entry point; `cmd/reconcile` is reserved for a future reconciliation command.

## Prerequisites

- Go (use the version specified by `go.mod`).
- Docker and Docker Compose.
- Goose v3 for applying migrations.
- `vacuum` only when editing or regenerating the OpenAPI specification.

### Install Goose

```bash
go install github.com/pressly/goose/v3/cmd/goose@v3.29.0
$(go env GOPATH)/bin/goose -version
```

Ensure `$(go env GOPATH)/bin` is on your `PATH` to invoke `goose` directly, or use its absolute path.

### Install vacuum

```bash
go install github.com/daveshanley/vacuum@v0.39.1
$(go env GOPATH)/bin/vacuum version
```

The generated OpenAPI bundle is committed to the repository, so `vacuum` is not needed to run the application.

## Running the Application

### 1. Configure Environment

From the repository root:

```bash
cp .env.example .env
```

Review `.env` for the local database connection and optional `APP_PORT`. The `.env` file should remain excluded from version control.

### 2. Start PostgreSQL

```bash
docker compose up -d
docker compose ps
```

The local development configuration uses PostgreSQL at `127.0.0.1:5433`.

### 3. Apply Database Migrations

Load the application environment in the terminal:

```bash
set -a
source .env
set +a
```

Apply and inspect migrations:

```bash
goose -dir migrations postgres "$DATABASE_URL" up
goose -dir migrations postgres "$DATABASE_URL" status
```

To roll back the latest migration in a controlled development environment:

```bash
goose -dir migrations postgres "$DATABASE_URL" down
```

**Warning:** Migration rollback can delete data. Review the migration before using `down`, especially outside a disposable local environment.

### 4. Start the API

With PostgreSQL running and migrations applied:

```bash
go run ./cmd/api
```

The server uses `DATABASE_URL` and listens on `APP_PORT` (default `8080`). It handles `SIGINT` and `SIGTERM` with graceful shutdown.

### 5. Verify Health

```bash
curl -i http://localhost:8080/health
```

Expected response: `200 OK`, with body `OK`.

The health endpoint checks HTTP availability; PostgreSQL connectivity is checked at application startup.

## API Endpoints

| Method | Endpoint | Status | Description |
|---|---|---|---|
| `GET` | `/health` | Implemented | HTTP health check |
| `GET` | `/openapi.yaml` | Implemented | Embedded bundled specification |
| `GET` | `/swagger` | Implemented | Swagger UI |
| `POST` | `/api/v1/accounts` | Implemented | Create a customer account |
| `GET` | `/api/v1/accounts/{id}` | In progress | Retrieve a customer account |
| `POST` | `/api/v1/accounts/{id}/deposits` | Planned | Deposit funds |
| `POST` | `/api/v1/transfers` | Planned | Transfer funds atomically |

### Create an Account

```http
POST /api/v1/accounts
Content-Type: application/json
Idempotency-Key: create-account-001
```

Request body:

```json
{
  "initial_balance_cents": 10000
}
```

`initial_balance_cents` is optional and defaults to zero. Negative amounts are rejected. Amounts are integer EUR cents.

Example successful response (`201 Created`):

```json
{
  "id": 2,
  "currency": "EUR",
  "balance_cents": 10000
}
```

The account ID is database-generated and will vary. Positive initial funding is transferred from the internal system account and recorded as balanced ledger entries in the same database transaction.

Try it locally:

```bash
curl -i -X POST http://localhost:8080/api/v1/accounts \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: create-account-demo-001' \
  -d '{"initial_balance_cents":10000}'
```

To create an account with a zero balance:

```bash
curl -i -X POST http://localhost:8080/api/v1/accounts \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: create-account-demo-002' \
  -d '{}'
```

### Idempotent Retries

Repeat the first request with **the same key and the same amount**:

```bash
curl -i -X POST http://localhost:8080/api/v1/accounts \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: create-account-demo-001' \
  -d '{"initial_balance_cents":10000}'
```

The API returns `201 Created` with the original account ID and response data. It does **not** create another account or post funding a second time.

Reusing that key with a different amount produces `409 Conflict`:

```bash
curl -i -X POST http://localhost:8080/api/v1/accounts \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: create-account-demo-001' \
  -d '{"initial_balance_cents":20000}'
```

Example error:

```json
{
  "error": "idempotency key already used with a different request"
}
```

A missing `Idempotency-Key`, invalid JSON, an unsupported JSON shape, or a negative initial balance produces `400 Bad Request`.

## API Documentation

The application serves a bundled **OpenAPI 3.0.3** specification and **Swagger UI**:

| URL | Description |
|---|---|
| `http://localhost:8080/swagger` | Interactive documentation |
| `http://localhost:8080/openapi.yaml` | Standalone bundled OpenAPI document |

The source specification is organized under `api/openapi.yaml`, `api/paths/`, and `api/schemas/`, with external `$ref` references. `api/openapi.bundle.yaml` is generated and embedded into the Go binary.

### Regenerate the OpenAPI Bundle

The OpenAPI specification is maintained in modular YAML files under `api/`.

The bundled specification (`api/openapi.bundle.yaml`) is committed to Git and embedded in the Go application using `go:embed`. **Redocly is not required to run the application or view the API documentation.**

After modifying the OpenAPI source files, regenerate the bundle using Docker:

```bash
docker run --rm \
  -v "$(pwd):/work" \
  -w /work \
  redocly/cli:latest \
  bundle api/openapi.yaml \
  --output api/openapi.bundle.yaml
```

Commit the generated bundle alongside the updated source files.

Swagger UI is available at `http://localhost:8080/swagger`, and the bundled OpenAPI specification is available at `http://localhost:8080/openapi.yaml`.

Swagger UI loads JavaScript and CSS from a CDN, so an internet connection is required to render the interface.

## Testing

The project uses Go's standard `testing` package. Tests cover HTTP validation, repository behavior, account-creation service behavior, idempotency, ledger posting, concurrent requests, and documentation endpoints.

### 1. Start the Test Database

The integration tests **require `TEST_DATABASE_URL`**. They intentionally do not fall back to `DATABASE_URL`, to avoid running database tests against the development database.

Start PostgreSQL if it is not already running:

```bash
docker compose up -d
```

Load the application environment:

```bash
set -a
source .env
set +a
```

For the supplied local development configuration, use:

```bash
export TEST_DATABASE_URL='postgres://bank:bank_dev_password@127.0.0.1:5433/bank_api_test?sslmode=disable'
```

The test database `bank_api_test` must exist. If it has not been created yet, create it using the configured PostgreSQL superuser (the local Compose setup uses `bank`):

```bash
docker compose exec -T postgres psql -U bank -d bank_api -c 'CREATE DATABASE bank_api_test;'
```

**Note:** The `postgres` service name in the command above assumes that the Compose service is named `postgres`. If your `docker-compose.yml` uses another service name, substitute it (check `docker compose ps`). If your Linux user cannot access the Docker socket, prefix Docker commands with `sudo` or configure Docker access appropriately.

**Safety:** Never point `TEST_DATABASE_URL` at a development database containing valuable data, or at a production database.

### 2. Run All Tests

```bash
go test ./... -count=1
```

Run with the race detector:

```bash
go test -race ./... -count=1
```

Run static analysis:

```bash
go vet ./...
```

### 3. Run Focused Tests

HTTP account handler tests (no PostgreSQL required):

```bash
go test ./internal/account -run '^TestHTTPHandler_' -v -count=1
```

Documentation endpoint tests:

```bash
go test ./api/... -v
```

Account service integration tests (require `TEST_DATABASE_URL`):

```bash
go test ./internal/account -run '^TestPostgresService_' -v -count=1
```

### Test Isolation

Integration tests connect to the dedicated `bank_api_test` database. Each test creates a uniquely named PostgreSQL schema, applies the Goose migrations inside it, and drops the schema during cleanup. This allows independent test execution without modifying application data in `bank_api`.

The integration tests cover successful account creation, optional funding, invalid requests, idempotency replay and conflicts, concurrent use of the same key, and concurrent funding from the shared system account.

### Manual Financial Integrity Check

After creating an account with a positive initial balance, inspect its ledger entries in PostgreSQL:

```sql
SELECT
    t.id AS transaction_id,
    t.idempotency_key,
    le.account_id,
    a.account_type,
    le.amount_cents
FROM transactions t
JOIN ledger_entries le ON le.transaction_id = t.id
JOIN accounts a ON a.id = le.account_id
WHERE t.idempotency_key = 'create-account-demo-001'
ORDER BY le.account_id;
```

A correctly funded account has one negative system entry and one equal positive customer entry. Verify their sum:

```sql
SELECT
    t.id AS transaction_id,
    COUNT(le.id) AS entries_count,
    SUM(le.amount_cents) AS total_cents
FROM transactions t
JOIN ledger_entries le ON le.transaction_id = t.id
WHERE t.idempotency_key = 'create-account-demo-001'
GROUP BY t.id;
```

Expected for a positive initial balance: `entries_count = 2` and `total_cents = 0`.

## Development Approach

Features are developed incrementally, with an emphasis on:

1. Defining the API contract and financial invariants.
2. Implementing business logic and persistence behind clear interfaces.
3. Writing unit, integration, and concurrency tests.
4. Updating OpenAPI documentation and regenerating the bundle.
5. Updating the README and making focused Git commits.

The priority is a reliable and maintainable banking API rather than unnecessary infrastructure complexity.