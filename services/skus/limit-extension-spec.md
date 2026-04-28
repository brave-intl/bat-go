# Self-Service Linking Limit Extension — Specification

## Motivation

Users who subscribe to Brave products with TLV2 credentials (e.g. Brave VPN, Leo) encounter a
hard default cap of 10 simultaneously linked devices. When they hit that cap, the only recourse
today is a support ticket.

The core user ask is **full autonomy over their own activations**: a subscriber should be able to
add device slots on their own schedule without waiting on support, up to a product-defined
ceiling. The ceiling exists to bound abuse (one subscription funding a shared pool), not to
gate legitimate multi-device use.

## Goals

- Let a paying subscriber increase their own device limit in the browser, without a support ticket.
- Make the extension self-contained: no operator action, no manual DB change, no new purchase.
- Keep the ceiling meaningful: cap both the number of extensions and the cadence so a single
  subscription cannot be scaled arbitrarily.
- Preserve operator authority: support can still set the limit directly via `SetLinkingLimit` at
  any value, independent of the self-service counter.
- Keep policy tunable without a skus deploy: slots/cadence/cap are supplied per-request by the
  caller, so operations that bump them only need a subscriptions deploy.

## Non-goals

- **Browser UI** — detecting the limit, surfacing the extension prompt, and retrying credential
  fetch after extension are a separate client-side workstream.
- **Purchasing additional activations** — a paid upgrade flow (vector 3) is a separate product
  decision; this feature deliberately leaves room for it without blocking on it.
- **Unlimited activations** — the 25-device ceiling (10 default + 5 extensions × 3 slots) is
  intentional. Subscribers who need more can contact support.
- **Per-user configurability of the cap** — the cap is a subscriptions-side constant; changing it
  requires a deploy of subscriptions, not a per-account setting.
- **Support revocation of extension ability** — no dedicated mechanism yet. Support can max out
  `num_self_extensions` as a workaround; a proper flag is a follow-up.

---

## Architecture

Responsibility is split across two services:

| Concern | Owned by |
|---|---|
| Public HTTP endpoint, subscriber JWT auth, ownership (subxID ↔ subscription match) | **subscriptions** |
| Product-type gate (TLV2 only), resolve orderID + orderItemID from subscription | **subscriptions** |
| Policy values (slots per grant, cadence, lifetime cap) | **subscriptions** |
| Error → HTTP status mapping for the browser | **subscriptions** |
| Row-level lock on `order_items`, guards under lock, atomic write | **bat-go/skus** |
| Defensive precondition checks (order paid, item is TLV2, order exists) | **bat-go/skus** |
| Wire error codes (`errorCode` strings) | **bat-go/skus**, consumed by subscriptions |

Flow:

```
Browser ──JWT──▶ subscriptions
                     │  (looks up sub, validates ownership + IsTLV2,
                     │   fetches order to resolve orderItemID,
                     │   builds policy)
                     │
                     ▼  POST /v1/orders/{orderID}/credentials/items/{itemID}/batches/extend
                     │  body: { slots_per_extension, min_seconds_between_extensions, max_extensions }
                     │  auth: PaymentAPIToken
                     ▼
                 bat-go/skus
                     │  (BeginTx → LockForUpdate → guards under policy → ApplyExtension → Commit)
                     │
                     └─► typed error (wire: errorCode + Retry-After + retry_after_seconds)
```

---

## Rules (subscriptions-side policy)

| Rule | Value | Source |
|------|-------|--------|
| Slots granted per extension | 3 | `extensionSlotsPerGrant` in `subscription.go` |
| Rate limit | Once per 30 days | `extensionMinIntervalSeconds` in `subscription.go` |
| Pre-condition | Available slots (limit − active) must be < 3 | enforced by skus under the lock |
| Lifetime cap | 5 self-service extensions per order item | `extensionMaxPerItem` in `subscription.go` |
| Total activations ceiling | 10 base + 5×3 = **25** | |
| Guard evaluation order | cap → rate limit → slots available | skus service method |

These values are hard-coded constants in subscriptions today. Runtime configurability is
deliberately deferred — raise it in PR review if/when operations needs it.

---

## Ways `max_active_batches_tlv2_creds` can increase

There are four distinct vectors. Only vector 2 touches the new tracking columns.

1. **Support sets limit directly** — arbitrary absolute value via `set-linking-limit`. Does not
   increment `num_self_extensions` or update `last_self_extension_at`. The user's
   self-service rate-limit clock keeps ticking from their last self-extension; a support
   bump does not reset it. This is intentional — support actions and self-service have
   independent counters, so a support bump never grants the user an extra self-extension
   immediately. (Runbook implication: if support wants the user to be able to self-extend
   right away, they need to clear `last_self_extension_at` themselves.)

2. **User self-service extension** — +3 slots via the new endpoint, subject to all rules above.
   Increments `num_self_extensions` and sets `last_self_extension_at`.

3. **User purchases additional activations** — future, product-defined increment. Does not touch
   the self-service tracking columns.

4. **Policy-level cap** — 5 extensions × 3 slots over baseline. Policy is supplied per-request by
   subscriptions; change it by deploying a new constant value in subscriptions.

Because vectors 1 and 3 use arbitrary increments, `num_self_extensions` cannot be derived from
`max_active_batches_tlv2_creds`. It must be tracked explicitly.

---

## Database migration (0076)

```sql
-- up
ALTER TABLE order_items
  ADD COLUMN num_self_extensions    INT         NOT NULL DEFAULT 0 CHECK (num_self_extensions >= 0),
  ADD COLUMN last_self_extension_at TIMESTAMPTZ;

-- down
ALTER TABLE order_items
  DROP COLUMN num_self_extensions,
  DROP COLUMN last_self_extension_at;
```

---

## bat-go/skus primitive

### Model changes (`model/model.go`)

New fields on `OrderItem` — `json:"-"` (internal, not exposed in the order API response):

```go
NumSelfExtensions   int        `db:"num_self_extensions"    json:"-"`
LastSelfExtensionAt *time.Time `db:"last_self_extension_at" json:"-"`
```

New Go errors:

```go
ErrExtensionRateLimited    Error = "model: extension rate limited"
ErrExtensionNotNeeded      Error = "model: extension not needed"
ErrExtensionCapReached     Error = "model: extension cap reached"
ErrInvalidExtensionPolicy  Error = "model: invalid extension policy"
```

Typed error carrying the server-observed retry window (matches `ErrExtensionRateLimited` under
`errors.Is`):

```go
type ExtensionRateLimitedError struct {
    RetryAfter time.Duration
}
```

Request body shape (caller supplies policy per-call):

```go
type ExtensionPolicy struct {
    SlotsPerExtension           int `json:"slots_per_extension"`
    MinSecondsBetweenExtensions int `json:"min_seconds_between_extensions"`
    MaxExtensions               int `json:"max_extensions"`
}
```

`ExtensionPolicy.Validate()` enforces defensive upper bounds (`extensionPolicyMaxSlots=100`,
`extensionPolicyMaxExtensions=1000`, `extensionPolicyMaxIntervalSecs=1 year`) to protect skus
from buggy callers. It does not express product policy.

Wire error codes — emitted as `errorCode` on every error response so callers can discriminate:

```go
ExtensionCodeMalformedBody       = "malformed_body"
ExtensionCodeInvalidPolicy       = "invalid_extension_policy"
ExtensionCodeOrderNotFound       = "order_not_found"
ExtensionCodeOrderNotPaid        = "order_not_paid"
ExtensionCodeUnsupportedCredType = "unsupported_cred_type"
ExtensionCodeCapReached          = "extension_cap_reached"
ExtensionCodeRateLimited         = "extension_rate_limited"
ExtensionCodeNotNeeded           = "extension_not_needed"
```

Status response shape (for `CountBatches`, extended to expose the extension tracking fields the
UI needs):

```go
type BatchesStatus struct {
    Limit               int        `json:"limit"`
    Active              int        `json:"active"`
    NumSelfExtensions   int        `json:"num_self_extensions"`
    LastSelfExtensionAt *time.Time `json:"last_self_extension_at"`
}
```

### Repository changes (`storage/repository/order_item.go`)

`LockForUpdate` — `SELECT ... FOR UPDATE` so concurrent self-service requests serialize at the
DB row level:

```sql
SELECT * FROM order_items WHERE id = $1 FOR UPDATE
```

`ApplyExtension` — atomic update that sets the new limit, increments the counter, and records
the timestamp:

```sql
UPDATE order_items
SET max_active_batches_tlv2_creds = $2,
    num_self_extensions           = num_self_extensions + 1,
    last_self_extension_at        = NOW()
WHERE id = $1
```

`SetMaxActiveBatches` is unchanged. Support direct limit changes (vector 1) do not touch
`num_self_extensions` or `last_self_extension_at`.

### Service method (`service.go`)

```go
func (s *Service) ExtendLinkingLimit(ctx context.Context, orderID, itemID uuid.UUID, policy model.ExtensionPolicy) error
```

Logic:

```
0.  policy.Validate()                                → ErrInvalidExtensionPolicy (caller bug)
1.  getOrderFullTx(ctx, rawDB, orderID)              → ErrOrderNotFound if missing
2.  order.IsPaid()                                   → ErrOrderNotPaid (pre-lock snapshot)
3.  len(order.Items) == 0                            → ErrInvalidOrderNoItems
4.  Find itemID in order items                       → ErrOrderItemNotFound
5.  item.IsCredTLV2()                                → ErrUnsupportedCredType
6.  Begin transaction (RawDB().BeginTxx)
7.  orderItemRepo.LockForUpdate(ctx, tx, item.ID)    ← row lock
8.  orderRepo.Get(ctx, tx, orderID).IsPaid()         ← TOCTOU re-check inside tx
9.  tlv2Repo.UniqBatches(ctx, tx, orderID, itemID, now, now) → activeCount
10. effectiveLimit = locked.MaxActiveBatchesTLV2CredsOrDefault()
11. locked.NumSelfExtensions >= policy.MaxExtensions → ErrExtensionCapReached
12. locked.LastSelfExtensionAt != nil &&
    now.Sub(*locked.LastSelfExtensionAt) < policy.MinInterval()
                                                     → &ExtensionRateLimitedError{RetryAfter: remaining}
13. available = effectiveLimit − activeCount
    available >= policy.SlotsPerExtension            → ErrExtensionNotNeeded
14. orderItemRepo.ApplyExtension(ctx, tx, locked.ID, effectiveLimit + policy.SlotsPerExtension)
15. Commit
```

The `FOR UPDATE` lock in step 7 serializes concurrent requests — the second request blocks
until the first commits and then sees the updated limit and extension count.

Ownership is **not** checked here. subscriptions is trusted to have validated ownership via the
subscriber JWT before calling.

### HTTP endpoint

```
POST /v1/orders/{orderID}/credentials/items/{itemID}/batches/extend
```

- **Auth**: `authMwr` (shared `PaymentAPIToken`, same as `CountBatches` and other endpoints
  subscriptions already calls)
- **Request body**: `{"slots_per_extension": int, "min_seconds_between_extensions": int, "max_extensions": int}`
- **Success response**: `200 OK`, empty JSON object `{}`

Error responses carry `errorCode` (the wire discriminator) on every error, plus `Retry-After`
header and `retry_after_seconds` in `data` for the rate-limited case:

| Wire `errorCode` | HTTP status | Notes |
|---|---|---|
| `malformed_body` | 400 | |
| `invalid_extension_policy` | 400 | Caller supplied out-of-bounds values |
| `order_not_found` | 404 | Includes `ErrInvalidOrderNoItems` and `ErrOrderItemNotFound` |
| `order_not_paid` | 402 | |
| `unsupported_cred_type` | 400 | Item is not TLV2 |
| `extension_cap_reached` | 403 | `NumSelfExtensions >= policy.MaxExtensions` |
| `extension_rate_limited` | 429 | + `Retry-After` header, + `data.retry_after_seconds` |
| `extension_not_needed` | 400 | Free slots already ≥ `policy.SlotsPerExtension` |
| (none) | 499 | `context.Canceled` |
| (none) | 504 | `context.DeadlineExceeded` |
| (none) | 500 | Unexpected internal error |

### Router registration (`controllers.go`)

```go
cr.Method(http.MethodPost, "/items/{itemID}/batches/extend",
    metricsMwr("ExtendLinkingLimit", authMwr(handlers.AppHandler(credh.ExtendLinkingLimit))))
```

---

## subscriptions public API

### Constants (`pkg/api/subscription.go`)

```go
const (
    extensionSlotsPerGrant      = 3
    extensionMinIntervalSeconds = 30 * 24 * 60 * 60
    extensionMaxPerItem         = 5
)
```

Yields the 10 + 5×3 = 25-activation ceiling.

### Service method (`pkg/api/subscription.go`)

```go
func (s *Server) extendLinkingLimit(ctx context.Context, subxID, subID uuid.UUID) error
```

Logic:

```
1. repositoryModule.getSubBySubxID(ctx, subxID, subID)  ← ownership check (row-level)
2. sub.OrderID.Valid                                    → errSubNotRecognised if missing
3. prodSet.ByID(sub.ProductID).IsTLV2()                 → errSKUsUnsupportedCredType
4. orderModule.FetchOrder(sub.OrderID)                  ← resolve order items
5. len(order.Items) == 1                                → errSKUsExtensionBundleOrder (defensive)
6. Build ExtensionPolicy from constants above
7. orderModule.ExtendLinkingLimit(orderID, Items[0].ID, policy)
```

Step 5 protects against silent mismatch once bundles (multi-item orders) arrive. The assumption
that TLV2 subscriptions are single-item today is from the existing skus comment in
`services/skus/controllers.go`.

### Client (`pkg/api/order.go`)

`orderModule.ExtendLinkingLimit` POSTs to the skus endpoint, parses the response, and maps the
wire `errorCode` to typed subscriptions-side errors:

| skus wire code | subscriptions Go error |
|---|---|
| `extension_rate_limited` | `*skusExtensionRateLimitedError{RetryAfter}` |
| `extension_cap_reached` | `errSKUsExtensionCapReached` |
| `extension_not_needed` | `errSKUsExtensionNotNeeded` |
| `order_not_found` | `errSKUsOrderNotFound` |
| `order_not_paid` | `errSKUsOrderNotPaid` |
| `unsupported_cred_type` | `errSKUsUnsupportedCredType` |
| `invalid_extension_policy`, `malformed_body` | `errSKUsInvalidExtensionPolicy` (caller bug) |
| (missing / unrecognised) | fallback by HTTP status |

Plus, before the wire-code switch:

- HTTP 499 (`StatusClientClosedConn`) → `model.ErrClientClosedConn`
- HTTP 504 (`StatusGatewayTimeout`) → `errSKUsDeadlineExceeded`

Rate-limit retry is extracted preferring `data.retry_after_seconds`, falling back to the
`Retry-After` header, defaulting to 1 second.

### HTTP endpoint

```
POST /v1/subscriptions/{subscriptionID}/credentials/batches/extend
```

- **Auth**: subscriber JWT (`authContextKey` → `authClaims.SubscriberID`)
- **Request body**: none (subscriptions owns policy, supplies it to skus internally)
- **Success response**: `200 OK`, empty JSON object `{}`

Error mapping to HTTP (subscriptions-side, what the browser sees):

| subscriptions Go error | HTTP status | Wire `errorCode` |
|---|---|---|
| `*skusExtensionRateLimitedError` | 429 | `extension_rate_limited` (+ `Retry-After`, + `data.retry_after_seconds`) |
| `errSubNotRecognised`, `errSubscriptionNotFound`, `errSKUsOrderNotFound` | 404 | (none — collapses "not yours" and "doesn't exist" as a BOLA guard) |
| `errSKUsOrderNotPaid` | 402 | `order_not_paid` |
| `errSKUsUnsupportedCredType` | 400 | `unsupported_cred_type` |
| `errSKUsExtensionCapReached` | 403 | `extension_cap_reached` |
| `errSKUsExtensionNotNeeded` | 400 | `extension_not_needed` |
| `errSKUsExtensionBundleOrder` | 400 | (none — internal assertion) |
| `ErrClientClosedConn` | 499 | (none) |
| `context.DeadlineExceeded`, `errSKUsDeadlineExceeded` | 504 | (none) |
| (default) | 500 | (none — only unexpected errors are logged at error level) |

### Router registration (`pkg/api/subscription.go`)

```go
r.HandleFunc("/{subscriptionID}/credentials/batches/extend",
    handlers.AppHandler(h.extendLinkingLimit).ServeHTTP).
    Methods(http.MethodPost).
    Name("ExtendLinkingLimit")
```

Registered under the existing subscriber-JWT middleware.

---

## Status endpoint changes

`GET /v1/orders/{orderID}/credentials/batches/count` (skus, existing) now returns the extension
tracking fields so the UI can show/hide the "request more activations" button:

```json
{
  "limit": 10,
  "active": 7,
  "num_self_extensions": 2,
  "last_self_extension_at": "2026-03-15T12:34:56Z"
}
```

`last_self_extension_at` is null when no self-service extension has been applied. The
corresponding subscriptions-side `batCredBatchesReport` struct has matching fields; existing
callers continue to work since the extra fields are additive.

---

## Tests

### bat-go primitive

- **Handler** (`handler/cred_test.go`, `TestCred_ExtendLinkingLimit`): table-driven cases for
  `invalid_orderID`, `invalid_itemID`, `malformed_body`, `invalid_policy`, `context_canceled`,
  `deadline_exceeded`, `order_not_found`, `order_not_paid`, `unsupported_cred_type`,
  `extension_not_needed`, `extension_cap_reached`, `rate_limited` (asserts
  `Retry-After: 42` + `data.retry_after_seconds: 42`), `internal_error`, `success`. Every
  error case also asserts the wire `errorCode`.

- **Service** (`service_nonint_test.go`, `TestService_ExtendLinkingLimit`): one case per guard
  using `MockOrderItem` + `FnLockForUpdate` + `FnApplyExtension`, verifying each guard fires
  independently and that `ApplyExtension` is not called when a guard rejects. Policy values
  are supplied per-test.

### subscriptions public API

- **Handler** (`handler_subscription_test.go`, `TestSubHandler_extendLinkingLimit`): every
  row of the error-mapping table above against a mocked `subSvc`. Asserts wire `errorCode`,
  HTTP status, and — for the rate-limited case — both the `Retry-After` header and
  `data.retry_after_seconds`. Includes a sub-second-clamps-to-1 case.

- **Service** (`subscription_test.go`, `TestServer_extendLinkingLimit`): each early-exit
  guard (sub lookup error, `errSubscriptionNotFound`, `errSubNotRecognised`, prod parse
  error, prod set miss, non-TLV2, order fetch error, multi-item bundle, zero-item order,
  skus-error pass-through), plus the happy path which freezes the wire contract by asserting
  the policy values handed to `orderModule.ExtendLinkingLimit` match the constants in
  `subscription.go` (3 / 30d / 5).

- **Client** (`order_test.go`, `TestOrderModule_ExtendLinkingLimit`): every wire `errorCode`
  → typed-error mapping, plus `Retry-After` extraction (body field, header fallback, 1s
  default), `504` → `errSKUsDeadlineExceeded`, `499` → `model.ErrClientClosedConn`, unknown
  errorCode + status fallback paths, and a happy path that asserts the outgoing URL,
  method, auth header, and serialised request body.

---

## Out of scope (flagged during spec review)

Tracked here so they don't get lost; each needs a separate discussion with product/support:

- **`access_forbidden` wire code**: product spec names this; we currently return 404 for both
  "not yours" and "doesn't exist" as a BOLA guard. Options: keep 404 + set
  `errorCode: "access_forbidden"` on it, or split 403/404. Needs product input.
- **Support revocation of extension ability**: spec calls for a way to disable self-service
  extension for a given user without touching `max_active_batches_tlv2_creds`. No mechanism
  today. Proper fix would be a schema flag (e.g. `self_extensions_revoked_at timestamptz NULL`);
  the current workaround is bumping `num_self_extensions` to the cap.
- **Client-side UI** — browser detects the limit error on credential fetch, surfaces the
  extension option, calls the subscriptions endpoint, and retries. Separate workstream.
- **Purchase flow (vector 3)** — separate workstream. The DB columns and endpoint design
  accommodate it when the time comes.
- **Support CLI** — tooling changes live in `tools/skus/cmd/skus.go` on this branch; coverage
  against the product spec (manual increment / reset) to be verified with support.
