# Self-Service Linking Limit Extension — Specification

## Overview

TLV2 orders have a default limit of 10 simultaneously linked
devices (`max_active_batches_tlv2_creds`). This feature allows users to request additional
device slots on a time-based cadence without requiring support intervention.

## Rules

| Rule | Value |
|------|-------|
| Slots granted per extension | 3 |
| Rate limit | Once per 30 days |
| Pre-condition | Available slots (limit − active) must be < 3 |
| Lifetime cap | 25 self-service extensions per order item (`selfExtensionCap` in `service.go`) |
| Guard evaluation order | cap → rate limit → slots available |

---

## Ways `max_active_batches_tlv2_creds` can increase

There are four distinct vectors. Only vector 2 touches the new tracking columns.

1. **Support sets limit directly** — arbitrary absolute value via `set-linking-limit`. Does not
   increment `num_self_extensions` or update `last_self_extension_at`.

2. **User self-service extension** — +3 slots via the new endpoint, subject to all rules above.
   Increments `num_self_extensions` and sets `last_self_extension_at`.

3. **User purchases additional activations** — future, product-defined increment. Does not touch
   the self-service tracking columns.

4. **Global hard cap** — 25 self-service extensions. This is a service-level constant, not stored
   per item. Change it by deploying a new constant value.

Because vectors 1 and 3 use arbitrary increments, `num_self_extensions` cannot be derived from
`max_active_batches_tlv2_creds`. It must be tracked explicitly.

---

## Database migration (0077)

```sql
-- up
ALTER TABLE order_items
  ADD COLUMN num_self_extensions    INT         NOT NULL DEFAULT 0,
  ADD COLUMN last_self_extension_at TIMESTAMPTZ;

-- down
ALTER TABLE order_items
  DROP COLUMN num_self_extensions,
  DROP COLUMN last_self_extension_at;
```

---

## Model changes (`model/model.go`)

New fields on `OrderItem` — `json:"-"` (internal, not exposed in the order API response):

```go
NumSelfExtensions   int        `db:"num_self_extensions"    json:"-"`
LastSelfExtensionAt *time.Time `db:"last_self_extension_at" json:"-"`
```

New error constants:

```go
ErrExtensionRateLimited    Error = "model: extension rate limited"
ErrExtensionSlotsAvailable Error = "model: extension not needed, slots available"
ErrExtensionCapReached     Error = "model: extension cap reached"
```

New constants:

```go
// in model/model.go
const (
    ExtensionSlots       = 3
    ExtensionMinInterval = 30 * 24 * time.Hour
)

// in service.go (unexported — raise by deploying a new constant value)
const selfExtensionCap = 25
```

---

## Repository changes (`storage/repository/order_item.go`)

### New: `LockForUpdate`

Fetches the item under `SELECT … FOR UPDATE` so concurrent self-service requests serialize at
the DB row level:

```go
LockForUpdate(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.OrderItem, error)
```

```sql
SELECT * FROM order_items WHERE id = $1 FOR UPDATE
```

### New: `ApplyExtension`

Sets the new limit, increments the counter, records the timestamp:

```go
ApplyExtension(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, newLimit int) error
```

```sql
UPDATE order_items
SET max_active_batches_tlv2_creds = $2,
    num_self_extensions           = num_self_extensions + 1,
    last_self_extension_at        = NOW()
WHERE id = $1
```

### No change to `SetMaxActiveBatches`

Support direct limit changes (vector 1) do not touch `num_self_extensions` or
`last_self_extension_at`.

---

## Service method (`service.go`)

```go
func (s *Service) ExtendLinkingLimit(ctx context.Context, orderID, itemID uuid.UUID) error
```

Logic:

```
0.  validateOrderMerchantAndCaveats(ctx, orderID) ← ownership/BOLA check
      ErrOrderNotFound                          → 404 (bubble through)
      errMerchantMismatch / errLocationMismatch
      errUnexpectedSKUCvt / errInvalidMerchant  → ErrOrderForbidden (403)
1.  getOrderFullTx(ctx, rawDB, orderID)         → ErrOrderNotFound if missing
2.  order.IsPaid()                              → ErrOrderNotPaid
3.  Find itemID in order items                  → ErrOrderItemNotFound
4.  item.IsCredTLV2()                           → ErrUnsupportedCredType
5.  Begin transaction (RawDB().BeginTxx)
6.  orderItemRepo.LockForUpdate(ctx, tx, item.ID)   ← re-fetch under row lock
7.  orderRepo.Get(ctx, tx, orderID).IsPaid()    ← TOCTOU re-check inside tx
8.  tlv2Repo.UniqBatches(ctx, tx, orderID, item.ID, now, now) → activeCount
9.  effectiveLimit = locked.MaxActiveBatchesTLV2CredsOrDefault()
10. locked.NumSelfExtensions >= selfExtensionCap → ErrExtensionCapReached
11. locked.LastSelfExtensionAt != nil &&
    now.Sub(*locked.LastSelfExtensionAt) < ExtensionMinInterval
                                                → ErrExtensionRateLimited
12. available = effectiveLimit − activeCount
      available >= ExtensionSlots               → ErrExtensionSlotsAvailable
13. orderItemRepo.ApplyExtension(ctx, tx, item.ID, effectiveLimit + ExtensionSlots)
14. Commit
```

The `FOR UPDATE` lock in step 6 serializes concurrent self-service requests — the second
request blocks until the first commits and then sees the updated limit and extension count.

---

## API endpoint

```
POST /v1/orders/{orderID}/credentials/items/{itemID}/batches/extend
```

- **Auth**: `authMwr` (client credential — user-initiated from the browser)
- **Request body**: none
- **Success response**: `200 OK`, empty JSON object `{}`

### Error mapping

| Error | HTTP status |
|-------|-------------|
| `ErrOrderForbidden` | 403 Forbidden |
| `ErrOrderNotFound` / `ErrInvalidOrderNoItems` / `ErrOrderItemNotFound` | 404 Not Found |
| `ErrOrderNotPaid` | 402 Payment Required |
| `ErrUnsupportedCredType` | 400 Bad Request |
| `ErrExtensionCapReached` | 403 Forbidden |
| `ErrExtensionRateLimited` | 429 Too Many Requests + `Retry-After: 2592000` |
| `ErrExtensionSlotsAvailable` | 400 Bad Request |
| `context.Canceled` | 499 Client Closed Connection |
| `context.DeadlineExceeded` | 504 Gateway Timeout |

### Router registration (`controllers.go`)

```go
cr.Method(http.MethodPost, "/items/{itemID}/batches/extend",
    metricsMwr("ExtendLinkingLimit", authMwr(handlers.AppHandler(credh.ExtendLinkingLimit))))
```

---

## Tests

### Handler unit tests (`handler/cred_test.go`)

Table-driven (`TestCred_ExtendLinkingLimit`). Cases:

- `invalid_orderID`, `invalid_itemID`
- `order_forbidden` (403)
- `context_canceled`, `deadline_exceeded`
- `order_not_found`, `order_not_paid`, `unsupported_cred_type`
- `extension_cap_reached` (403)
- `rate_limited` (429, asserts `Retry-After: 2592000` response header)
- `slots_already_available` (400)
- `internal_error` (500)
- `success` (200)

### Service unit tests (`service_test.go`)

One test per validation rule using `MockOrderItem` with `FnLockForUpdate` and `FnApplyExtension`,
verifying each guard fires independently and that `ApplyExtension` is not called when a guard
rejects.

---

## Out of scope

- **Client-side UI** — browser detects the limit error on credential fetch, surfaces the
  extension option, calls this endpoint, and retries. Separate workstream.
- **Purchase flow (vector 3)** — separate workstream. The DB columns and endpoint design
  accommodate it when the time comes.
- **Support CLI command** — no operator action needed for self-service extensions.
