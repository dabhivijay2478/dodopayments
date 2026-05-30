# AGENTS.md — Invoice & Payment Service

Instructions for AI agents (Cursor, Claude, etc.) working in this repository.

---

## Project Summary

Go 1.22 **Invoice & Payment Service** for the Dodo Payments take-home:

- **API** (`cmd/api`) — Chi router, `POST /bootstrap` (first key, no auth), API key CRUD, customers, invoices, payments, webhooks
- **Mock PSP** (`cmd/mock-psp`) — `POST /charge` on port 9090
- **Postgres 16** — GORM AutoMigrate (no raw SQL migration files)
- **Money** — `int64` cents only; never `float64` in money paths
- **Critical path** — `POST /invoices/{id}/pay` with idempotency, `FOR UPDATE`, 10s PSP timeout

Primary docs: [DESIGN.md](DESIGN.md), [API.md](API.md), [README.md](README.md), [AI_USAGE.md](AI_USAGE.md).

---

## Commands

```bash
# Sync dependencies (run after go.mod changes or before first build)
go mod tidy

# Run full stack
docker compose up --build

# Build
go build ./...

# Run all tests (requires Postgres via DATABASE_URL in .env)
cp .env.example .env
go test ./tests/... -v -timeout 60s

# Run individual tests
go test ./tests/... -run TestConcurrentPayments -v -timeout 30s
go test ./tests/... -run TestIdempotency -v -timeout 30s
go test ./tests/... -run TestPSPTimeout -v -timeout 30s
go test ./tests/... -run TestPSPNetworkError -v -timeout 30s

# Vet
go vet ./...
```

---

## Repository Layout

```
cmd/api/                 main server
cmd/mock-psp/            mock payment processor
internal/config/         env config (.env auto-loaded via godotenv)
internal/db/             connect, pgcrypto, AutoMigrate
internal/models/         all GORM models
internal/middleware/     API key auth
internal/handlers/       HTTP handlers (apikeys, customers, invoices, payments, webhooks)
internal/payment/        PSP HTTP client
internal/webhook/        async HMAC delivery + retries
tests/
  helpers_test.go        shared fixtures + router builder
  concurrency_test.go    10 concurrent pays → 1 success
  idempotency_test.go    replay + conflict detection
  psp_failure_test.go    timeout + network error handling
```

---

## Non-Negotiable Rules

1. **No floats for money** — use `int64` `*_cents` fields only
2. **Server computes invoice total** — never accept client `total_cents`
3. **Pay only when invoice `state == open`**
4. **Idempotency-Key** required on pay; reuse → same response; different body → 422
5. **PSP timeout** — `context.WithTimeout(10*time.Second)` on pay handler
6. **Webhooks** — always `go webhook.Dispatch(...)`; never block API response
7. **Pay flow** — short DB tx (lock + insert `pending`) → commit → call PSP → update attempt + invoice
8. **Errors** — JSON `{ "error": { "code", "message" } }` for API errors; pay failures use 402 + `{ status, code }`
9. **pgcrypto** — `CREATE EXTENSION IF NOT EXISTS pgcrypto` before AutoMigrate
10. **Do not edit** assignment deliverable tone in `AI_USAGE.md` unless user asks

---

## Invoice States

```
draft → open (finalize) → paid (pay success)
draft → void
open → void
```

Terminal: `paid`, `void`, `uncollectible` (reserved).

---

## Payment Handler Order

1. Idempotency lookup (succeeded / failed / pending → 200 / 402 / 202)
2. Transaction: `SELECT ... FOR UPDATE`, verify `open`, check no in-flight attempt, insert `pending`, commit
3. PSP call with 10s context timeout
4. On success: attempt `succeeded`, invoice `paid`, webhook `invoice.paid`
5. On failure: attempt `failed`, invoice stays `open`, 402, webhook `invoice.payment_failed`

---

## Mock PSP Tokens

| Token | Behavior |
|-------|----------|
| `tok_success` | ~100ms → succeeded |
| `tok_insufficient_funds` | failed |
| `tok_card_declined` | failed |
| `tok_timeout` | sleep 30s (API aborts at 10s) |
| `tok_network_error` | HTTP 500 |

---

## Test Files

| File | Tests | Purpose |
|------|-------|---------|
| `tests/helpers_test.go` | (shared) | DB setup, fixture creation, router builder, HTTP call helper |
| `tests/concurrency_test.go` | `TestConcurrentPayments` | N=10 goroutines, exactly 1 success, no double-charge |
| `tests/idempotency_test.go` | `TestIdempotency`, `TestIdempotencyConflict` | Replay returns cached response; different body → 422 |
| `tests/psp_failure_test.go` | `TestPSPTimeout`, `TestPSPNetworkError` | Timeout/500 → 402, invoice stays open, attempt failed |

---

## When Making Changes

- Match existing patterns in `handlers/helpers.go` (`respondJSON`, `respondError`)
- Scope changes minimally; do not add out-of-scope features
- Update DESIGN.md if behavior changes affect failure-mode answers
- Run `go test ./tests/...` after payment handler changes

---

## Out of Scope (Discuss in DESIGN Only)

- Refunds, partial payments, multi-currency, tax, email, OAuth, production rate limiting

---

## Submission Checklist

- [ ] `docker compose up` works on a clean machine, no manual steps
- [ ] No floats anywhere in the money path
- [ ] All 5 tests pass: `go test ./tests/... -v -timeout 60s`
- [ ] `tok_timeout` does not hang the endpoint
- [ ] DESIGN.md answers all 7 sections
- [ ] AI_USAGE.md is honest and specific
- [ ] Video link in README.md
