# Bank Account API

A Go-based banking API developed as a backend engineering take-home assignment.

## Overview

The application provides three core operations:

- Create a bank account with a zero or specified initial balance.
- Deposit money into an existing account.
- Transfer money atomically between accounts.

## Architecture

The project follows a modular monolith architecture, with PostgreSQL as the transactional data store.

### Design goals

- ACID transactions and concurrency-safe transfers.
- Double-entry ledger for financial traceability.
- Idempotent financial operations.
- Consistent account balances.
- Internal reconciliation and automated testing.
- Reproducible local development using Docker Compose.

## Technology Stack

- Go
- PostgreSQL
- Docker Compose
- Go standard library (`net/http`, `testing`)

## Project Status

**Work in progress.**

The architecture and financial integrity requirements are being established. Implementation, setup instructions, API documentation, and test coverage will be added as development progresses.

## Running the Application

Instructions will be added once the application is implemented.

## Prerequisites

The following tools are required for local development:

- **Go** — application development and testing
- **Docker & Docker Compose** — PostgreSQL container
- **Goose v3** — database schema migrations

### Install Goose

```bash
go install github.com/pressly/goose/v3/cmd/goose@v3.29.0
```

Verify the installation:

```bash
goose -version
```

### Environment Configuration

Create a local environment file:

```bash
cp .env.example .env
```

The `.env.example` file documents the required environment variables. The `.env` file is excluded from version control.

### Start PostgreSQL

```bash
docker compose up -d
```

Verify that PostgreSQL is running:

```bash
docker compose ps
```

PostgreSQL is accessible locally at `127.0.0.1:5433` by default.

### Database Migrations

Database schema changes are managed using [Goose](https://github.com/pressly/goose).

After starting PostgreSQL, load the environment variables:

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

Migrations are versioned and tracked in PostgreSQL through the `goose_db_version` table. Running `up` multiple times is safe: already applied migrations are skipped.

**Note:** Rolling back migrations can delete data. The `down` command is intended for controlled development use and should not be executed against production without reviewing the migration.

### Running the API

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

Verify that the HTTP server is running:

```bash
curl -i http://localhost:8080/health
```

Expected response:

```text
HTTP/1.1 200 OK

OK
```

The `/health` endpoint currently checks HTTP server availability. Database connectivity is verified during application startup.