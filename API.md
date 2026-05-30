# API Reference

## Base URL

`http://localhost:8080` (Docker / local)

## Authentication

All routes except `GET /health` require:

```
Authorization: Bearer <api_key>
```

### How you get the **first** API key (bootstrap)

| Method | When to use |
|--------|-------------|
| **`POST /bootstrap`** (no auth) | **Recommended.** Works when no active API keys exist. Returns `api_key` in JSON. |
| **Docker logs** | Only when DB is empty on first `docker compose up` (`SEED_DATA=true`). |

```bash
curl -s -X POST http://localhost:8080/bootstrap
```

Response `201`:

```json
{
  "api_key": "sk_...",
  "authorization": "Bearer sk_...",
  "id": "...",
  "message": "First API key created. Store api_key now; it is not shown again."
}
```

If `409 already_bootstrapped`, an active key already exists — use `POST /api-keys` to rotate (with your current key) or `docker compose down -v` for a fresh DB.

Logs (optional): `docker compose logs api | grep "TEST API KEY"` — empty if the database was already seeded.

**Important:** `POST /api-keys` requires Bearer auth and is for **rotation only**.

### Rotate API keys (after bootstrap)

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api-keys` | Mint new key; optional `revoke_key_id` in body |
| `GET` | `/api-keys` | List keys (prefix + revoked status; no secrets) |
| `DELETE` | `/api-keys/{id}` | Revoke old key → **401** on next use |

Storage: `key_prefix` (first 11 chars) + SHA-256 `key_hash`. Full secret never stored after creation.

## Error Format

```json
{
  "error": {
    "code": "snake_case",
    "message": "Human readable message"
  }
}
```

Payment failures use a separate shape:

```json
{
  "status": "failed",
  "code": "card_declined"
}
```

---

## POST /bootstrap

**No authentication.** Creates the first business API key when no active (non-revoked) keys exist.

**Response `201`** — same shape as `POST /api-keys` (`api_key`, `authorization`, `id`, …)

**Response `409`** — `already_bootstrapped` if an active key already exists

```bash
curl -s -X POST http://localhost:8080/bootstrap
```

---

## GET /health

No auth.

**Response `200`**

```json
{"status":"ok"}
```

```bash
curl -s http://localhost:8080/health
```

---

## POST /api-keys

**Requires:** existing Bearer token (from bootstrap logs or a previous rotation).

Create a **new** business API key. Optional body revokes an old key in the same request. Does **not** replace bootstrap — use this only when you already have a working key.

**Body (optional)**

| Field | Type | Required |
|-------|------|----------|
| revoke_key_id | uuid string | no — revoke this key after creating the new one |

**Response `201`**

```json
{
  "id": "...",
  "key_prefix": "sk_abc1234",
  "api_key": "sk_...",
  "token_type": "Bearer",
  "authorization": "Bearer sk_...",
  "created_at": "...",
  "message": "Store api_key now. Revoked keys return 401 immediately."
}
```

```bash
curl -s -X POST http://localhost:8080/api-keys \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"revoke_key_id":"OLD_KEY_UUID"}'
```

---

## GET /api-keys

**Response `200`** — `{ "data": [{ "id", "business_id", "key_prefix", "revoked_at", "created_at" }], "count": N }` (no secrets)

```bash
curl -s http://localhost:8080/api-keys -H "Authorization: Bearer $API_KEY"
```

---

## DELETE /api-keys/{id}

Revokes the key (`revoked_at` set). That key stops working immediately.

**Response `200`**

```json
{"id":"...","revoked_at":"...","message":"API key revoked; requests using it will receive 401"}
```

**Errors:** `404` not_found, `409` already_revoked

```bash
curl -s -X DELETE http://localhost:8080/api-keys/KEY_UUID \
  -H "Authorization: Bearer $API_KEY"
```

---

## POST /customers

**Headers:** `Authorization`, `Content-Type: application/json`

**Body**

| Field | Type | Required |
|-------|------|----------|
| name | string | yes |
| email | string | yes |

**Response `201`** — Customer object (`id`, `business_id`, `name`, `email`, `created_at`, `updated_at`)

**Errors:** `400` bad_request, `401` unauthorized

```bash
curl -s -X POST http://localhost:8080/customers \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name":"Acme","email":"a@acme.com"}'
```

---

## GET /customers

**Response `200`**

```json
{"data": [...], "count": 1}
```

```bash
curl -s http://localhost:8080/customers -H "Authorization: Bearer $API_KEY"
```

---

## GET /customers/{id}

**Response `200`** — Customer object

**Errors:** `404` not_found

```bash
curl -s http://localhost:8080/customers/$ID -H "Authorization: Bearer $API_KEY"
```

---

## POST /invoices

**Body**

| Field | Type | Required |
|-------|------|----------|
| customer_id | uuid string | yes |
| due_date | string (YYYY-MM-DD or RFC3339) | yes |
| line_items | array | yes |
| line_items[].description | string | yes |
| line_items[].quantity | int64 | yes (>0) |
| line_items[].unit_amount_cents | int64 | yes (>0) |

Server computes `total_cents`. No client `total` field.

**Response `201`** — Invoice with `line_items`, `customer`, `state: draft`

```bash
curl -s -X POST http://localhost:8080/invoices \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"customer_id":"'$CID'","due_date":"2026-06-01","line_items":[{"description":"Item","quantity":1,"unit_amount_cents":1000}]}'
```

---

## GET /invoices

**Query:** `state` (optional) — filter by invoice state

**Response `200`**

```json
{"data": [...], "count": N}
```

```bash
curl -s "http://localhost:8080/invoices?state=open" -H "Authorization: Bearer $API_KEY"
```

---

## GET /invoices/{id}

**Response `200`** — Invoice with `line_items`, `customer`, `payment_attempts`

```bash
curl -s http://localhost:8080/invoices/$ID -H "Authorization: Bearer $API_KEY"
```

---

## POST /invoices/{id}/finalize

Draft → open.

**Response `200`** — Updated invoice

**Errors:** `409` invalid_transition

```bash
curl -s -X POST http://localhost:8080/invoices/$ID/finalize -H "Authorization: Bearer $API_KEY"
```

---

## POST /invoices/{id}/void

Draft or open → void.

**Response `200`**

**Errors:** `409` invalid_transition

```bash
curl -s -X POST http://localhost:8080/invoices/$ID/void -H "Authorization: Bearer $API_KEY"
```

---

## POST /invoices/{id}/pay

**Headers**

| Header | Required |
|--------|----------|
| Idempotency-Key | yes |
| Authorization | yes |
| Content-Type | application/json |

**Body**

| Field | Type | Required |
|-------|------|----------|
| card_token | string | yes |

**Responses**

| Status | Body |
|--------|------|
| 200 | `{"status":"succeeded","payment_attempt_id":"...","psp_reference":"..."}` |
| 202 | `{"status":"pending","message":"..."}` |
| 402 | `{"status":"failed","code":"..."}` |
| 409 | invalid_state (not open / concurrent) |
| 422 | idempotency_conflict |

```bash
curl -s -X POST http://localhost:8080/invoices/$ID/pay \
  -H "Authorization: Bearer $API_KEY" \
  -H "Idempotency-Key: unique-key-1" \
  -H "Content-Type: application/json" \
  -d '{"card_token":"tok_success"}'
```

---

## POST /webhook-endpoints

**Body:** `{ "url": "https://example.com/hooks" }` (must start with `http`)

**Response `201`:** `id`, `url`, `secret` (shown once), `created_at`

```bash
curl -s -X POST http://localhost:8080/webhook-endpoints \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://webhook.site/your-id"}'
```

---

## GET /webhook-endpoints

**Response `200`:** `{ "data": [{ "id", "url", "created_at" }], "count": N }` — no `secret`

```bash
curl -s http://localhost:8080/webhook-endpoints -H "Authorization: Bearer $API_KEY"
```

---

## Mock PSP Tokens

`POST http://localhost:9090/charge` (internal; API uses `PSP_BASE_URL`)

| Token | Result |
|-------|--------|
| tok_success | 200 succeeded (100ms) |
| tok_insufficient_funds | 200 failed `insufficient_funds` |
| tok_card_declined | 200 failed `card_declined` |
| tok_timeout | sleeps 30s then succeeded (API times out at 10s) |
| tok_network_error | 500 |
| other | 400 unknown token |

---

## Invoice States

| State | Terminal | Valid transitions |
|-------|----------|-------------------|
| draft | no | → open (finalize), → void |
| open | no | → paid (pay), → void |
| paid | yes | none |
| void | yes | none |
| uncollectible | yes | reserved (not implemented) |
