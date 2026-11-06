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