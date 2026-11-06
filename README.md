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
