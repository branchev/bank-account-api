# Bank Account API

A Go-based banking REST API developed as a backend engineering take-home assignment.

## Overview

The application is designed to provide three core operations:

- Create a bank account with a zero or specified initial balance.
- Deposit money into an existing account.
- Transfer money atomically between accounts.

The implementation prioritizes financial correctness, concurrency safety, maintainability, and testability.

## Architecture

The project follows a **modular monolith** architecture, with PostgreSQL as its transactional data store.

Business domains are logically separated within a single application, allowing future service decomposition when justified by actual requirements.

### Design Goals

- ACID transactions and concurrency-safe transfers.
- Double-entry ledger for financial traceability.
- Idempotent financial operations.
- Consistent account balances.
- Internal reconciliation and automated testing.
- Reproducible local development using Docker Compose.
- Versioned REST API with OpenAPI documentation.

## Technology Stack

- **Go** — application development.
- **PostgreSQL 17** — transactional data storage.
- **pgx** — PostgreSQL driver and connection pooling.
- **Goose** — database schema migrations.
- **Docker Compose** — local PostgreSQL environment.
- **Go standard library** — HTTP server, routing, testing, and embedded files.
- **OpenAPI 3.0.3** — API specification.
- **Swagger UI** — interactive API documentation.

## Project Status

**Work in progress.**

The following components are currently implemented:

- Go HTTP server with graceful shutdown.
- PostgreSQL connection pool and startup connectivity verification.
- Docker Compose configuration for PostgreSQL.
- Database schema and initial Goose migration.
- Accounts, transactions, and ledger entries tables.
- Initial system funding account.
- Database constraints and ledger immutability trigger.
- `GET /health` endpoint.
- Embedded OpenAPI specification.
- Swagger UI at `/swagger`.
- Automated tests for documentation endpoints.

The account creation, deposit, transfer, idempotency, financial posting, and reconciliation workflows are planned but not yet implemented.

## Architecture and Conventions

The project follows a **modular monolith** architecture, with clearly separated business domains and responsibilities.

The design prioritizes financial correctness, maintainability, testability, and the possibility of future service decomposition without introducing unnecessary distributed-system complexity.

### API Conventions

| Area | Convention |
|---|---|
| API versioning | `/api/v1` |
| Resource naming | RESTful, plural nouns (e.g. `/accounts`, `/transfers`) |
| Request/response format | JSON using `snake_case` field names |
| API specification | OpenAPI 3.0.3 |
| Interactive documentation | Swagger UI at `/swagger` |
| OpenAPI document | `/openapi.yaml` |
| Health check | `/health` (outside the versioned API) |
| Error handling | Consistent JSON error responses with appropriate HTTP status codes |
| Idempotency | `Idempotency-Key` HTTP header for financial write operations |

The `/api/v1` prefix applies to business endpoints. Health checks and API documentation remain outside the versioned API namespace.

### Go Conventions

- Follow idiomatic Go naming conventions (`MixedCaps` for identifiers).
- Keep HTTP handlers focused on transport concerns: request parsing, validation, and response serialization.
- Implement business rules in dedicated domain services.
- Encapsulate database access behind repositories or clearly defined persistence interfaces where appropriate.
- Pass `context.Context` through request-scoped operations.
- Prefer Go's standard library unless an external dependency provides clear value.
- Keep `cmd/api/main.go` focused on application initialization, dependency wiring, and server lifecycle management.

### Persistence and Financial Integrity

- **Database:** PostgreSQL.
- **Database driver:** `pgx`, using a connection pool.
- **Schema migrations:** Goose.
- **SQL tooling:** `sqlc` is planned for type-safe database queries.
- **Currency:** EUR.
- **Money representation:** Integer cents (`BIGINT` in PostgreSQL); floating-point arithmetic is not used for monetary values.
- **Transaction consistency:** Financial operations must execute atomically within PostgreSQL transactions.
- **Concurrency:** Account-level row locking and deterministic lock ordering are planned to protect concurrent balance modifications.
- **Ledger:** Financial movements will be recorded as immutable, signed ledger entries.
- **Balance consistency:** Account balances and corresponding ledger entries must be updated within the same database transaction.
- **Reconciliation:** Ledger and account balances will be checked for consistency.

The initial database migration establishes the required tables, constraints, and ledger immutability trigger. The financial posting workflows and their transactional guarantees will be implemented in subsequent development stages.

### Architecture Principles

- **Modular monolith first:** Business domains remain logically separated while sharing one deployment and database.
- **Single responsibility:** HTTP transport, business logic, and persistence have distinct responsibilities.
- **Financial correctness:** Atomicity, idempotency, and concurrency safety take priority over premature optimization.
- **Future decomposition:** Module boundaries should allow services to be extracted when justified by actual requirements.
- **API stability:** Public API versioning is independent of internal service boundaries.
- **Incremental development:** Features are implemented with tests and corresponding documentation updates.

### Project Structure

```text
bank-api/
├── api/
│   ├── handler.go
│   ├── handler_test.go
│   ├── openapi.yaml
│   └── swagger/
│       └── index.html
├── cmd/
│   ├── api/
│   │   └── main.go
│   └── reconcile/
├── internal/
│   ├── account/
│   ├── ledger/
│   ├── transfer/
│   └── platform/
│       └── database/
│           └── postgres.go
├── migrations/
├── docker-compose.yml
├── go.mod
├── go.sum
└── README.md
```

The `cmd/api` directory contains the REST API entry point, while the top-level `api` package contains the OpenAPI specification and documentation handlers.

The `internal` directory contains application-specific business modules and infrastructure code.

The `cmd/reconcile` directory is reserved for the planned reconciliation command.

## Prerequisites

The following tools are required for local development:

- **Go** — application development and testing.
- **Docker & Docker Compose** — PostgreSQL container.
- **Goose v3** — database schema migrations.

### Install Goose

```bash
go install github.com/pressly/goose/v3/cmd/goose@v3.29.0
```

Verify the installation:

```bash
goose -version
```

## Running the Application

### 1. Environment Configuration

Create a local environment file:

```bash
cp .env.example .env
```

The `.env.example` file documents the required environment variables. The `.env` file is excluded from version control.

### 2. Start PostgreSQL

```bash
docker compose up -d
```

Verify that PostgreSQL is running:

```bash
docker compose ps
```

PostgreSQL is accessible locally at `127.0.0.1:5433` by default, using the configuration provided in `.env.example`.

### 3. Database Migrations

Database schema changes are managed using [Goose](https://github.com/pressly/goose).

Load the environment variables:

```bash
set -a
source .env
set +a
```

Apply all pending migrations:

```bash
goose -dir migrations postgres "$DATABASE_URL" up
```

Check migration status:

```bash
goose -dir migrations postgres "$DATABASE_URL" status
```

To roll back the most recently applied migration:

```bash
goose -dir migrations postgres "$DATABASE_URL" down
```

Migrations are versioned and tracked in PostgreSQL through the `goose_db_version` table.

Running `up` multiple times is safe: already applied migrations are skipped.

**Note:** Rolling back migrations can delete data. The `down` command is intended for controlled development use and should not be executed against production without reviewing the migration.

### 4. Start the API

Ensure PostgreSQL is running and the database migrations have been applied.

Load the environment variables if they are not already available in your terminal:

```bash
set -a
source .env
set +a
```

Start the Go API:

```bash
go run ./cmd/api
```

The application connects to PostgreSQL using `DATABASE_URL` and starts an HTTP server on the port specified by `APP_PORT` (default: `8080`).

The server supports graceful shutdown on `SIGINT` and `SIGTERM`.

### 5. Verify the API

Check the HTTP server:

```bash
curl -i http://localhost:8080/health
```

Expected response:

```text
HTTP/1.1 200 OK

OK
```

The `/health` endpoint currently checks HTTP server availability. Database connectivity is verified during application startup.

## API Documentation

The API uses **OpenAPI 3.0.3** for its specification and **Swagger UI** for interactive documentation.

Once the application is running, the following endpoints are available:

| Endpoint | Description |
|---|---|
| `http://localhost:8080/swagger` | Interactive Swagger UI |
| `http://localhost:8080/openapi.yaml` | OpenAPI specification |
| `http://localhost:8080/health` | Application health check |

### Swagger UI

Open the following URL in your browser:

**http://localhost:8080/swagger**

Swagger UI displays the available operations, request and response schemas, and allows supported API requests to be executed interactively.

The current specification documents the implemented `GET /health` endpoint. Business endpoints will be added as their API contracts and implementations are developed.

### Embedded Documentation

The OpenAPI specification and Swagger UI HTML are embedded into the compiled Go binary using the standard library's `embed` package.

This removes the runtime dependency on the application's working directory for these files.

Swagger UI currently loads its JavaScript and CSS assets from a CDN, so an internet connection is required to display the interface.

## Testing

Tests use Go's standard `testing` package.

### Run All Tests

```bash
go test ./...
```

### Run Documentation Tests

```bash
go test ./api/... -v
```

The documentation tests verify that:

- `GET /openapi.yaml` returns HTTP 200 with the expected YAML content.
- `GET /swagger` returns HTTP 200 with the expected HTML content.

Additional unit, integration, financial integrity, and concurrency tests will be introduced alongside the corresponding business functionality.

## Planned API Endpoints

The following business endpoints are planned for implementation:

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/api/v1/accounts` | Create a bank account |
| `GET` | `/api/v1/accounts/{id}` | Retrieve account information |
| `POST` | `/api/v1/accounts/{id}/deposits` | Deposit money into an account |
| `POST` | `/api/v1/transfers` | Transfer money between accounts |

These endpoints are not yet available.

Financial write operations will use PostgreSQL transactions and idempotency controls to protect against duplicate execution and inconsistent balances.

## Development Approach

The project is developed incrementally, with each completed feature including:

- A clearly defined API contract.
- Business logic and persistence implementation.
- Appropriate automated tests.
- OpenAPI specification updates.
- README updates.
- A focused Git commit.

The priority is a reliable and maintainable banking API rather than unnecessary infrastructure complexity.