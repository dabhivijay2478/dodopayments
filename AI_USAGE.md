# AI Usage Disclosure

## Which AI Tools I Used and For What

- **Cursor (Claude agent mode):** Scaffolded the initial project structure — `go.mod`, `docker-compose.yml`, GORM model definitions, chi router wiring. Generated the first draft of handler boilerplate (`CreateCustomer`, `CreateInvoice`, `ListInvoices`). Drafted the ASCII state machine diagram in DESIGN.md.
- **Cursor Copilot (autocomplete):** Inline completions for repetitive patterns — chi route registration, `respondJSON`/`respondError` calls, JSON struct tags, test assertion boilerplate.
- **Claude (chat):** Asked it to explain Postgres `SELECT FOR UPDATE` semantics vs. advisory locks. Asked about `context.WithTimeout` propagation through `http.Client.Do`. Used it to draft the webhook retry schedule rationale.

## Three Decisions I Made Myself

1. **`SELECT FOR UPDATE` over optimistic locking.**
   - AI suggested a version column with conditional UPDATE (`WHERE version = ?`) and retry on conflict.
   - I chose `FOR UPDATE` because the pay endpoint is write-heavy on a single row under contention. Row locks give a deterministic 409 to losers without requiring clients to implement retry loops. The lock window is minimal (insert pending attempt + commit) before the PSP call happens outside the transaction.

2. **10-second PSP context timeout (not 30s or configurable).**
   - AI did not suggest a specific value; it generated `context.WithTimeout` with a placeholder.
   - I chose 10s because `tok_timeout` sleeps 30s — the timeout must be strictly less to avoid hanging. 10s gives the PSP reasonable time for real latency while protecting the API thread pool. Making it configurable adds complexity with no assignment benefit.

3. **Store full request body JSON on `payment_attempts` for idempotency conflict detection.**
   - AI initially only compared `card_token` in the handler code without persistence.
   - I chose to marshal `{"card_token":"..."}` into `request_body TEXT` on the row, so that idempotency conflict detection survives process restarts and works correctly even if the handler is stateless. This also future-proofs against adding fields (amount overrides, metadata) to the pay request.

## One Thing the AI Got Wrong

The initial scaffold used `gen_random_uuid()` as the GORM default for primary keys but did **not** enable the `pgcrypto` extension. GORM's `AutoMigrate` ran without error (it only creates tables/columns), but the first `INSERT` failed with:

```
ERROR: function gen_random_uuid() does not exist
```

This only surfaced on a **clean Postgres volume** — my local dev had the extension from a previous project. I fixed it by adding `db.DB.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto")` before `AutoMigrate` in `db.Connect()`. Verified by running `docker compose down -v && docker compose up --build` from scratch.
