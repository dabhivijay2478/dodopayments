# Invoice & Payment Service

## Demo Video

> 📹 [Loom link — add before submission]

## Language Choice

Go was chosen over Rust because of existing production experience with Go services, chi, and GORM. Goroutines fit async webhook delivery without extra infrastructure. GORM AutoMigrate keeps the assignment scope focused on correctness (payments, idempotency, locking) rather than migration tooling. The spec allows other languages; a correct, tested Go service was prioritized over an unfamiliar stack.

## Quick Start

```bash
git clone <repo-url>
cd <repo>
cp .env.example .env   # local API + tests use DATABASE_URL from here
docker compose up --build
```

On first boot, the API prints a test key:

```
=============================
TEST API KEY: sk_xxxx
=============================
```

Use it as `Authorization: Bearer <key>` on all routes except `GET /health`.

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | Postgres DSN (API and tests share this) | required — set in `.env` or environment |
| `PORT` | HTTP listen port | `8080` |
| `PSP_BASE_URL` | Mock PSP base URL | `http://localhost:9090` |
| `SEED_DATA` | Seed business + API key when `true` | `false` |

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

Postgres must be running and accessible via `DATABASE_URL` in `.env`:

```bash
cp .env.example .env
# Start Postgres (if not already running via docker compose):
docker compose up postgres -d
```

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
internal/handlers/   — HTTP handlers (customers, invoices, payments, webhooks)
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
