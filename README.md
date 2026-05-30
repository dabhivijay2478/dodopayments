# Invoice & Payment Service

## Demo Video

> 📹 [Loom link — add before submission]

## Language Choice

Go was chosen over Rust because of existing production experience with Go services, chi, and GORM. Goroutines fit async webhook delivery without extra infrastructure. GORM AutoMigrate keeps the assignment scope focused on correctness (payments, idempotency, locking) rather than migration tooling. The spec allows other languages; a correct, tested Go service was prioritized over an unfamiliar stack.

## Quick Start

```bash
git clone https://github.com/dabhivijay2478/dodopayments
cd dodopayments
go mod tidy              # sync go.sum with go.mod (required before first build)
cp .env.example .env     # local API + tests: DATABASE_URL uses host port 5433
docker compose up --build
```

`docker compose up` needs no manual DB setup: Postgres, API, and mock PSP start together. The API container talks to Postgres on the Docker network (`postgres:5432`). From your **host** (tests, `go run`), use **port 5433** in `.env` so you do not clash with another Postgres on 5432.

### First API key (bootstrap)

You **cannot** use `POST /api-keys` without already having a key. Get the first key in either way:

**Option A — API call (recommended, works in Postman):**

```bash
curl -s -X POST http://localhost:8080/bootstrap
```

No `Authorization` header. Returns `201` with `api_key` when no active keys exist.  
Postman: run **00 → First API Key - POST /bootstrap** — it saves `apiKey` automatically.

**Option B — Docker logs (only on first empty database):**

```bash
docker compose logs api | grep "TEST API KEY"
```

If this is empty, the DB was already seeded — use **Option A** or reset: `docker compose down -v && docker compose up --build`.

Use the key as: `Authorization: Bearer sk_...`

`POST /api-keys` is for **rotation** after you have a working key.

### Rotate API key (after you have a bootstrap key)

1. Create a new key (optionally revoke the old one in the same request):

```bash
curl -s -X POST http://localhost:8080/api-keys \
  -H "Authorization: Bearer $OLD_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"revoke_key_id":"OLD_KEY_ID"}'
```

2. Save the new `api_key` from the response and update Postman variable `apiKey`.
3. Or revoke separately: `DELETE /api-keys/{id}` — that key returns `401` on the next request.

List keys: `GET /api-keys` (shows `key_prefix`, `revoked_at`; no secrets).

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | Postgres DSN (API and tests share this) | required — set in `.env` or environment |
| `PORT` | HTTP listen port | `8080` |
| `PSP_BASE_URL` | Mock PSP base URL | `http://localhost:9090` |
| `SEED_DATA` | Seed business + API key when `true` | `false` |

### Local development (without Docker for the API)

```bash
go mod tidy
cp .env.example .env
docker compose up postgres mock-psp -d   # Postgres on localhost:5433
go run ./cmd/api
```

Run `go mod tidy` whenever you change `go.mod` or see `missing go.sum entry` during `docker compose build`.

## curl Examples

Replace `$API_KEY` with the key from docker logs. Replace UUIDs from previous responses.

### 1. Create customer

```bash
curl -s -X POST http://localhost:8080/customers \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name":"Acme Corp","email":"billing@acme.com"}'
```

Response `201`:
```json
{"id":"...","business_id":"...","name":"Acme Corp","email":"billing@acme.com","created_at":"...","updated_at":"..."}
```

### 2. Create invoice with line items

```bash
curl -s -X POST http://localhost:8080/invoices \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "customer_id": "CUSTOMER_UUID",
    "due_date": "2026-06-15",
    "line_items": [
      {"description":"Widget","quantity":2,"unit_amount_cents":1500},
      {"description":"Support plan","quantity":1,"unit_amount_cents":5000}
    ]
  }'
```

Response `201` — `total_cents` is `8000` (server-computed: 2×1500 + 1×5000).

### 3. Attempt payment — success (tok_success)

```bash
# First finalize the invoice (draft → open):
curl -s -X POST http://localhost:8080/invoices/INVOICE_UUID/finalize \
  -H "Authorization: Bearer $API_KEY"

# Then pay:
curl -s -X POST http://localhost:8080/invoices/INVOICE_UUID/pay \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: pay-$(uuidgen)" \
  -d '{"card_token":"tok_success"}'
```

Response `200`:
```json
{"status":"succeeded","payment_attempt_id":"...","psp_reference":"..."}
```

### 4. Attempt payment — failure (tok_card_declined)

```bash
curl -s -X POST http://localhost:8080/invoices/OPEN_INVOICE_UUID/pay \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: pay-decline-$(uuidgen)" \
  -d '{"card_token":"tok_card_declined"}'
```

Response `402`:
```json
{"status":"failed","code":"card_declined"}
```

Invoice remains `open` — client can retry with a new idempotency key.

---

## Testing

### Prerequisites

Postgres must be running and reachable at the host in `.env` (default **port 5433**):

```bash
go mod tidy
cp .env.example .env
# Start Postgres (if not already running):
docker compose up postgres -d
```

`DATABASE_URL` in `.env` should look like:

`postgres://postgres:postgres@localhost:5433/invoicedb?sslmode=disable`

### Run all tests

```bash
go test ./tests/... -v -timeout 60s
```

### Run individual tests

Each test is in its own file for clarity:

| File | Test | What it verifies |
|------|------|-----------------|
| `tests/concurrency_test.go` | `TestConcurrentPayments` | 10 goroutines fire pay simultaneously → exactly 1 succeeds (200), 9 get 409, invoice is `paid`, only 1 succeeded attempt |
| `tests/idempotency_test.go` | `TestIdempotency` | Same idempotency key sent twice → same 200 response, PSP called once, 1 DB row |
| `tests/idempotency_test.go` | `TestIdempotencyConflict` | Same key + different card_token → 422 `idempotency_conflict` |
| `tests/psp_failure_test.go` | `TestPSPTimeout` | PSP hangs 12s → API returns 402 `timeout` in <15s, invoice stays `open`, attempt `failed` |
| `tests/psp_failure_test.go` | `TestPSPNetworkError` | PSP returns 500 → 402 `network_error`, invoice stays `open`, attempt `failed` |

```bash
# Run only the concurrency test:
go test ./tests/... -run TestConcurrentPayments -v -timeout 30s

# Run only idempotency tests:
go test ./tests/... -run TestIdempotency -v -timeout 30s

# Run only PSP failure tests:
go test ./tests/... -run TestPSP -v -timeout 30s
```

### What the tests prove

- **No double-charges:** Concurrent payment attempts are serialized by `SELECT FOR UPDATE`; the DB + lock guarantee at most one successful attempt per invoice.
- **Idempotency safety:** Replaying the exact same request returns the cached result without hitting the PSP again.
- **Timeout resilience:** The 10-second context timeout prevents the API from hanging when the PSP is slow; the invoice is never left in a corrupted state.

---

## Project Structure

```
cmd/api/             — HTTP API entrypoint (chi router, seed, routes)
cmd/mock-psp/        — Mock PSP binary (POST /charge, token-based behavior)
internal/config/     — Loads .env + environment variables
internal/db/         — Postgres connection, pgcrypto, GORM AutoMigrate
internal/models/     — 8 GORM models (all money as int64 cents)
internal/middleware/ — Bearer API key auth middleware
internal/handlers/   — HTTP handlers (api keys, customers, invoices, payments, webhooks)
internal/payment/    — PSP HTTP client (10s timeout)
internal/webhook/    — Async HMAC-signed delivery with exponential retries
tests/               — Integration tests (separate files per concern)
  helpers_test.go    — Shared setup: DB, fixtures, router builder, HTTP helpers
  concurrency_test.go — 10 concurrent pay requests → 1 success
  idempotency_test.go — Replay + conflict detection
  psp_failure_test.go — Timeout + network error handling
```

## Architecture

```
Client ──▶ chi router ──▶ AuthMiddleware ──▶ Handler
                                                │
                                    ┌───────────┼───────────┐
                                    ▼           ▼           ▼
                               Postgres      PSP client   Webhook goroutine
                            (FOR UPDATE)    (10s timeout)  (HMAC + retries)
```

1. All requests pass through `AuthMiddleware`, which validates the bearer token via prefix lookup + SHA-256 hash comparison.
2. Invoice totals are always computed server-side from line items.
3. The pay handler uses a short DB transaction (lock + insert pending), commits, then calls the PSP externally.
4. PSP outcomes update the attempt + invoice state in separate DB calls (no transaction needed).
5. Webhooks are dispatched in background goroutines — never blocking the API response.

## Postman

Import `postman/Invoice-Payment-Service.postman_collection.json` (File import, not Raw text). Run **00 → First API Key - POST /bootstrap** first (sets `apiKey` automatically). Then **01 - Happy Path**.
