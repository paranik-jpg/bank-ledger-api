# Bank Ledger API

A REST API written in Go for user registration, JWT-based authentication, and atomic bank-account transfers backed by PostgreSQL. The project focuses on transaction safety: each transfer records a transfer row and corresponding debit/credit ledger entries while updating both account balances inside a single database transaction.

## Features

- **User registration** with bcrypt password hashing.
- **Login and JWT authentication** using signed access tokens.
- **Protected transfer endpoint** requiring a `Bearer` token.
- **Atomic money transfers** implemented with PostgreSQL transactions.
- **Ledger entries** recording debit and credit movements.
- **Concurrency-aware balance updates** with a deterministic account-ID lock/update order to reduce deadlock risk.
- **Database constraints and migrations** for schema integrity.
- **HTTP middleware** for request logging and panic recovery.
- **HTTP server timeouts** for read, write, and idle connections.
- **Docker Compose** setup for the API and PostgreSQL.
- **Automated tests** for transfer, service, and middleware behavior.

## Architecture

```mermaid
flowchart TD
    Client[HTTP Client] --> Router[HTTP Router]
    Router --> Recovery[Panic Recovery Middleware]
    Recovery --> Logger[Request Logging Middleware]
    Logger --> Public[Registration and Login]
    Logger --> Auth[JWT Authentication Middleware]
    Auth --> TransferHandler[Transfer Handler]
    Public --> UserService[User and Auth Services]
    TransferHandler --> TransferService[Transfer Service]
    UserService --> DB[(PostgreSQL)]
    TransferService --> DB
    DB --> Accounts[Accounts]
    DB --> Transfers[Transfers]
    DB --> Entries[Ledger Entries]
```

The handlers validate HTTP requests and translate results into HTTP responses. Services implement application rules, while the database layer owns SQL operations and the atomic transfer transaction.

## Tech Stack

- Go
- `net/http` for HTTP routing and server behavior
- PostgreSQL
- `github.com/jackc/pgx/v5` for PostgreSQL connectivity
- `github.com/golang-jwt/jwt/v5` for JWTs
- `golang.org/x/crypto/bcrypt` for password hashing
- `golang-migrate` migration files
- Docker and Docker Compose

## Prerequisites

For the containerized setup:

- Docker Engine
- Docker Compose plugin

For running the API directly:

- A Go toolchain compatible with `go.mod`
- PostgreSQL
- The `migrate` CLI if you want to run migrations with the Makefile

## Getting Started

### 1. Clone the repository

```bash
git clone <repository-url>
cd bank-ledger-api
```

Replace `<repository-url>` with the repository's actual Git URL.

### 2. Configure secrets before running

The application requires a PostgreSQL connection URL and a JWT signing secret. Do not commit real credentials or secrets to the repository.

For local development, create a `.env` file in the project root. The file is ignored by Git.

Generate a strong JWT secret:

```bash
openssl rand -hex 32
```

Configure the following variables in `.env`:

```dotenv
POSTGRES_USER=your_database_user
POSTGRES_PASSWORD=your_database_password
POSTGRES_DB=bank_ledger
DATABASE_URL=postgresql://your_database_user:your_database_password@localhost:5432/bank_ledger?sslmode=disable&TimeZone=Asia%2FKolkata
JWT_SECRET=your_generated_jwt_secret
```

Use your own local database credentials and the generated JWT secret. Keep `.env` private.

The application supports these environment variables:

|    Variable    |                Purpose                |                   Behavior                    |
|----------------|---------------------------------------|-----------------------------------------------|
|     `PORT`     |          HTTP listening port          |              Defaults to `:8080`              |
| `DATABASE_URL` |       PostgreSQL connection URL       |        Required unless `DB_URL` is set        |
|    `DB_URL`    | Alternative PostgreSQL connection URL |       Used when `DATABASE_URL` is unset       |
|  `JWT_SECRET`  | HMAC signing secret for access tokens | Required; must contain at least 32 characters |

The connection URL must be reachable from where the API runs. Inside Docker Compose, the database hostname is `postgres`; from the host machine, it is normally `localhost`.

### 3. Start with Docker Compose

Create the `.env` file described above, then start the services:

```bash
docker compose config --quiet
docker compose up --build
```

Compose starts PostgreSQL and waits for its health check before starting the API.

| Service | Host address | Purpose |
|---|---|---|
| API | `http://localhost:8080` | HTTP endpoints |
| PostgreSQL | `localhost:5432` | Persistent database |

The Compose file stores PostgreSQL data in the named volume `data-volume`.

Stop the services:

```bash
docker compose down
```

To also delete the database volume and its stored data:

```bash
docker compose down --volumes
```

**Warning:** removing the volume permanently deletes the local database contents. Changing `POSTGRES_PASSWORD` in `.env` does not automatically change the password of a database that has already been initialized in the existing volume.

### 4. Run locally without Docker (optional)

Start PostgreSQL and create a database and user. Then configure the connection settings in your shell:

```bash
export PORT=8080
export DATABASE_URL='postgresql://<user>:<password>@localhost:5432/bank_ledger?sslmode=disable&TimeZone=Asia%2FKolkata'
export JWT_SECRET="$(openssl rand -hex 32)"
```

Replace the URL placeholders with your local database credentials. URL-encode reserved characters in the username or password when placing them in a connection URL.

Apply the migrations using the `migrate` CLI:

```bash
make migrateup
```

The Makefile reads `DATABASE_URL` from the environment, or falls back to the `DATABASE_URL` entry in `.env` when the variable is unset.

Start the API:

```bash
go run ./cmd/api
```

## API Reference

All request and response bodies use JSON unless noted otherwise. Error responses generally have this shape:

```json
{
  "error": "Description of the problem"
}
```

### `POST /api/v1/users`

Registers a new user. Passwords are hashed using bcrypt before storage.

Request:

```json
{
  "username": "nikhil",
  "email": "nikhil@example.com",
  "password": "your-password"
}
```

Validation performed by the handler includes a username of at least 3 characters, a parseable email address, and a password of at least 6 characters. Username and email must be unique.

Success: `201 Created`

Example response:

```json
{
  "username": "nikhil",
  "email": "nikhil@example.com",
  "created_at": "2026-10-13 19:30:00"
}
```

The timestamp above is illustrative; the actual value is generated by the server.

Common errors:

- `400 Bad Request` — malformed JSON or invalid fields
- `409 Conflict` — username or email already exists
- `500 Internal Server Error` — unexpected server/database error

Example:

```bash
curl -i -X POST http://localhost:8080/api/v1/users \
  -H 'Content-Type: application/json' \
  -d '{
    "username": "nikhil",
    "email": "nikhil@example.com",
    "password": "your-password"
  }'
```

### `POST /api/v1/sessions`

Authenticates a user and returns a signed JWT access token. The configured token lifetime is 24 hours.

Request:

```json
{
  "username": "nikhil",
  "password": "your-password"
}
```

Success: `200 OK`

```json
{
  "access_token": "<jwt-token>",
  "expires_at": "<expiry timestamp>"
}
```

Common errors:

- `400 Bad Request` — malformed JSON or missing credentials
- `401 Unauthorized` — invalid username/password
- `500 Internal Server Error` — unexpected error

Example:

```bash
curl -i -X POST http://localhost:8080/api/v1/sessions \
  -H 'Content-Type: application/json' \
  -d '{
    "username": "nikhil",
    "password": "your-password"
  }'
```

Copy the returned `access_token` for the protected endpoint below.

### `POST /api/v1/transfers`

Creates a transfer between two existing accounts. This endpoint requires a valid JWT in the `Authorization` header.

Request:

```json
{
  "from_account_id": 1,
  "to_account_id": 2,
  "amount": 500
}
```

The amount is represented as an integer in the currency's smallest unit (for example, paise for INR), rather than a floating-point value. Ensure both accounts use compatible currencies; currency compatibility is not currently checked by the transfer service.

Success: `200 OK`

```json
{
  "transfer_id": 1,
  "from_account_id": 1,
  "to_account_id": 2,
  "amount": 500,
  "from_balance": 4500,
  "to_balance": 2500,
  "created_at": "<transfer timestamp>"
}
```

Example:

```bash
curl -i -X POST http://localhost:8080/api/v1/transfers \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <jwt-token>' \
  -d '{
    "from_account_id": 1,
    "to_account_id": 2,
    "amount": 500
  }'
```

Common errors:

- `400 Bad Request` — non-positive amount, identical source/destination accounts, or insufficient funds
- `401 Unauthorized` — missing, malformed, invalid, or expired token
- `404 Not Found` — one or both account IDs do not exist
- `405 Method Not Allowed` — method other than POST
- `500 Internal Server Error` — unexpected internal error

> **Important:** the current HTTP API exposes registration, login, and transfers. Account creation/listing methods exist in the database layer, but there are no HTTP endpoints for creating or listing accounts in the current router. Seed or create test accounts through a controlled development workflow before testing transfers.

### `GET /api/v1/panic-test`

A testing endpoint that deliberately panics to exercise recovery middleware. It is not a business endpoint and should be removed or disabled in production.

## How a Transfer Works

The database layer executes the transfer as one PostgreSQL transaction:

1. Validate that the amount is positive and the source and destination account IDs differ.
2. Insert a row into `transfers`.
3. Insert a negative ledger entry for the source account.
4. Insert a positive ledger entry for the destination account.
5. Update the two account balances in ascending account-ID order.
6. Reject the transfer if either account is missing or a resulting balance is negative.
7. Commit the transaction only if all operations succeed.

If any step fails, the transaction is rolled back, so the transfer record, entries, and balance changes do not partially persist. Updating accounts in a deterministic ID order reduces deadlock risk when concurrent transfers touch the same accounts in opposite directions.

The `accounts` table also has a database-level check preventing negative balances. The application uses PostgreSQL transactions at `READ COMMITTED` isolation.

## Database Schema

| Table | Purpose |
|---|---|
| `users` | Usernames, bcrypt password hashes, email addresses, creation timestamps |
| `accounts` | Account owner, integer balance, currency, creation timestamp |
| `transfers` | Transfer history with source, destination, amount, timestamp |
| `entries` | Ledger movements; negative values represent debits and positive values represent credits |

The migrations also add indexes for account owners, account entries, and source/destination transfer lookups. Foreign keys enforce relationships between accounts and ledger records.

## Project Structure

```text
.
├── .github/
│   └── workflows/
│       └── test.yml                 # CI test workflow
├── cmd/
│   └── api/
│       └── main.go                  # App setup, middleware, routes, server
├── db/
│   └── migration/                   # Versioned up/down SQL migrations
├── internal/
│   ├── database/                    # PostgreSQL models, queries, transactions
│   ├── handler/                     # HTTP handlers and JSON responses
│   ├── middleware/                  # JWT auth, request logging, recovery
│   └── service/                     # Registration, auth, transfer rules
├── Dockerfile
├── docker-compose.yml
├── Makefile
├── go.mod
└── go.sum
```

## Development

Format the Go source:

```bash
gofmt -w ./cmd ./internal
```

Run tests:

```bash
go test ./...
```

Run tests with coverage:

```bash
go test -v -cover ./...
```

The Makefile also provides:

```bash
make test
make run
make migrateup
make migratedown
```

Review the Makefile before using migration targets: its `DB_URL` is currently set to a development connection string.

## Security and Production Notes

This project is a learning/reference implementation and should not be exposed as a production banking service without further work.

- **Secrets:** replace the committed development database credentials and fallback JWT secret. Use environment/secret management and rotate any real secret that has been committed.
- **Database credentials in logs:** the current application logs the database URL at startup. Avoid logging connection URLs that contain credentials.
- **JWT configuration:** use a high-entropy secret, validate token claims and expected signing algorithm, and consider key rotation and revocation requirements.
- **Authorization model:** the transfer handler records the authenticated username as the initiator, but it does not currently verify that the user owns the source account or is authorized to operate on either account.
- **Input limits:** request bodies are not currently bounded with `http.MaxBytesReader`, and JSON decoding does not explicitly reject trailing JSON values.
- **Transfer semantics:** currency compatibility and idempotency keys are not implemented. A client retry after an uncertain network result could submit the same transfer twice.
- **Ledger reconciliation:** the ledger entries are recorded transactionally, but the project does not currently expose a ledger-history endpoint or perform reconciliation.
- **Database startup:** the app verifies PostgreSQL connectivity, but migrations are a separate operation and must be applied before the API is used.
- **Operational readiness:** add graceful shutdown, structured/redacted logging, metrics, tracing, backup/recovery procedures, and integration tests for concurrent transfers before production use.
- **Testing endpoint:** remove or protect `/api/v1/panic-test` in production.

## License

See [`LICENSE`](LICENSE) for the project's license.
