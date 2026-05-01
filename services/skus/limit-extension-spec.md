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
- Keep policy iterable without a skus deploy: slots/cadence/cap/per-product overrides live in
  subscriptions; skus only enforces a DB sanity ceiling that bounds blast radius.

## Non-goals

- **Browser UI** — detecting the limit, surfacing the extension prompt, and retrying credential
  fetch after extension are a separate client-side workstream.
- **Purchasing additional activations** — a paid upgrade flow (vector 3) is a separate product
  decision; this feature deliberately leaves room for it without blocking on it.
- **Unlimited activations** — the per-product policy ceiling is intentional. Subscribers who
  need more contact support.
- **Support revocation of extension ability** — no dedicated mechanism yet. Support can max out
  `num_self_extensions` as a workaround; a proper flag is a follow-up.

---

## Architecture

The split is intentional: **subs owns all policy**, **skus is thin storage**. skus enforces only
the row-level DB sanity ceiling (1000) — every subjective rule (rate-limit window, slots per
grant, max per item, at-limit precondition) is evaluated in subscriptions.

This lets ops iterate policy with a subs deploy and lets skus stay generic.

| Concern | Owned by |
|---|---|
| Public HTTP endpoint, subscriber JWT auth, ownership (subxID ↔ subscription match) | **subscriptions** |
| Product-type gate (TLV2 only), resolve orderID + orderItemID from subscription | **subscriptions** |
| Per-product policy values (slots per grant, cadence, lifetime cap), at-limit gate | **subscriptions** |
| Error → HTTP status mapping for the browser | **subscriptions** |
| Optimistic-concurrency CAS write, atomic `num_self_extensions++` and `last_self_extension_at = NOW()` | **bat-go/skus** |
| DB sanity ceiling (`max_active_batches_tlv2_creds <= 1000`) | **bat-go/skus** (CHECK constraint) |
| Defensive precondition checks (order paid, item is TLV2, order exists) | **bat-go/skus** |

Flow:

```
Browser ──JWT──▶ subscriptions
                     │  (looks up sub, validates ownership + IsTLV2,
                     │   fetches order to resolve itemID,
                     │   selects per-product policy)
                     │
                     │  GET /credentials/batches/count → state {limit, active, num_self_extensions, last_self_extension_at}
                     │  evaluate policy locally:
                     │    not at limit         → 422 not_at_limit
                     │    num_self_ext >= cap  → 422 max_per_item
                     │    within rate window   → 429 rate_limited
                     │
                     ▼  POST /v1/orders/{orderID}/credentials/items/{itemID}/batches/extend
                     │  body: { expected_last_self_extension_at, new_limit }
                     │  auth: PaymentAPIToken
                     │
                     ▼  skus CAS write:
                     │    UPDATE WHERE last_self_extension_at IS NOT DISTINCT FROM expected
                     │    rows == 0 → 409 extension_conflict (subs refetches, retries once)
                     │    new_limit > 1000 → 422 extension_invalid_limit (CHECK violation)
                     │
                     └─► success or typed conflict for retry
```

---

## Rules (subs-owned policy)

| Rule | Default | Source |
|------|---------|--------|
| Slots granted per extension | 3 | `defaultExtensionPolicy.SlotsPerGrant` |
| Rate limit | Once per 30 days | `defaultExtensionPolicy.MinIntervalSeconds` |
| Pre-condition | `active >= limit` (must be at limit; no top-up math) | `evaluateExtensionPolicy` |
| Lifetime cap | 5 self-service extensions per item | `defaultExtensionPolicy.MaxPerItem` |
| Per-product overrides | none yet | `extensionPolicies` map keyed on product id |
| skus DB sanity ceiling | `max_active_batches_tlv2_creds <= 1000` | CHECK constraint (migration 0076) |
| Guard order in subs | not-at-limit → max-per-item → rate-limit | `evaluateExtensionPolicy` |

The "under 3 free slots" math is intentionally gone. The new contract is binary: a user is
**at limit** or they are not, and an extension grants exactly `SlotsPerGrant` more — independent
of how many they happen to be using right now.

Per-product overrides live in subs (`extensionPolicies` map). Adding one is a subs deploy; no
schema change.

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

2. **User self-service extension** — `+SlotsPerGrant` via the new endpoint, subject to all
   subs-owned rules above. Increments `num_self_extensions` and sets `last_self_extension_at`
   in the same atomic CAS update. Cannot exceed the DB sanity ceiling of 1000.

3. **User purchases additional activations** — future, product-defined increment. Does not touch
   the self-service tracking columns.

4. **Policy-level cap** — defined per-product in subs. Change it by deploying subs.

Because vectors 1 and 3 use arbitrary increments, `num_self_extensions` cannot be derived from
`max_active_batches_tlv2_creds`. It must be tracked explicitly.

---

## Database migration (0076)

```sql
-- up
ALTER TABLE order_items
  ADD COLUMN num_self_extensions    INT         NOT NULL DEFAULT 0 CHECK (num_self_extensions >= 0),
  ADD COLUMN last_self_extension_at TIMESTAMPTZ,
  ADD CONSTRAINT order_items_max_active_batches_tlv2_creds_sanity
    CHECK (max_active_batches_tlv2_creds IS NULL OR max_active_batches_tlv2_creds <= 1000);

-- down
ALTER TABLE order_items
  DROP CONSTRAINT IF EXISTS order_items_max_active_batches_tlv2_creds_sanity,
  DROP COLUMN num_self_extensions,
  DROP COLUMN last_self_extension_at;
```

The CHECK constraint is the only piece of policy that lives in skus. It bounds the absolute
maximum a single row can ever hold, independent of any caller bug or compromised caller.

---

## bat-go/skus primitive

### Model changes (`model/model.go`)

`OrderItem` exposes the new tracking columns over JSON so the order-fetch path can return them:

```go
NumSelfExtensions   int        `db:"num_self_extensions"    json:"num_self_extensions"`
LastSelfExtensionAt *time.Time `db:"last_self_extension_at" json:"last_self_extension_at"`
```

CAS write payload (caller supplies the version token + the absolute new limit):

```go
type ExtensionWrite struct {
    ExpectedLastSelfExtensionAt *time.Time `json:"expected_last_self_extension_at"`
    NewLimit                    int        `json:"new_limit"`
}
```

`ExpectedLastSelfExtensionAt == nil` means "expected the row to have never been extended."

State exposed by `CountBatches` (additive — existing fields unchanged):

```go
type BatchesStatus struct {
    Limit               int        `json:"limit"`
    Active              int        `json:"active"`
    NumSelfExtensions   int        `json:"num_self_extensions"`
    LastSelfExtensionAt *time.Time `json:"last_self_extension_at"`
}
```

Errors and wire codes — minimal set for thin storage:

```go
ErrExtensionInvalidLimit Error = "model: extension new limit invalid"
ErrExtensionConflict     Error = "model: extension version conflict"
```

```go
ExtensionCodeMalformedBody       = "malformed_body"
ExtensionCodeOrderNotFound       = "order_not_found"
ExtensionCodeOrderNotPaid        = "order_not_paid"
ExtensionCodeUnsupportedCredType = "unsupported_cred_type"
ExtensionCodeConflict            = "extension_conflict"
ExtensionCodeInvalidLimit        = "extension_invalid_limit"
```

No rate-limit / cap / not-needed codes — those judgments are subs's job.

### Repository changes (`storage/repository/order_item.go`)

`LockForUpdate` is gone. Concurrency is now CAS, not row-level locking.

`ApplyExtensionCAS` — single atomic UPDATE that doubles as the optimistic-concurrency check:

```sql
UPDATE order_items
SET max_active_batches_tlv2_creds = $2,
    num_self_extensions           = num_self_extensions + 1,
    last_self_extension_at        = NOW()
WHERE id = $1
  AND last_self_extension_at IS NOT DISTINCT FROM $3
```

If `RowsAffected() == 0`, the version token has changed under us — return `ErrExtensionConflict`
so subs can refetch and retry. `IS NOT DISTINCT FROM` is null-safe so the "never extended" case
(`expected = NULL`, current `IS NULL`) matches correctly.

If the new `max_active_batches_tlv2_creds` violates the CHECK constraint, Postgres returns a
`pq.Error` with code `23514`; skus maps that to `ErrExtensionInvalidLimit`.

`SetMaxActiveBatches` is unchanged. Support direct limit changes (vector 1) do not touch
`num_self_extensions` or `last_self_extension_at`.

### Service method (`service.go`)

```go
func (s *Service) ExtendLinkingLimit(ctx context.Context, orderID, itemID uuid.UUID, write model.ExtensionWrite) error
```

Logic — purely defensive checks plus the CAS write. **No policy.**

```
1.  write.NewLimit <= 0 || write.NewLimit > 1000 → ErrExtensionInvalidLimit
2.  getOrderFullTx(rawDB, orderID)               → ErrOrderNotFound if missing
3.  order.IsPaid()                               → ErrOrderNotPaid
4.  Find itemID in order items                   → ErrOrderItemNotFound
5.  item.IsCredTLV2()                            → ErrUnsupportedCredType
6.  orderItemRepo.ApplyExtensionCAS(rawDB, item.ID, write.ExpectedLastSelfExtensionAt, write.NewLimit)
       rows == 0       → ErrExtensionConflict
       CHECK violation → ErrExtensionInvalidLimit
```

Ownership is **not** checked here. subs is trusted to have validated ownership via the
subscriber JWT before calling.

There is no transaction, no row lock, and no read-then-write. The CAS UPDATE is the entire
mutation.

### HTTP endpoint

```
POST /v1/orders/{orderID}/credentials/items/{itemID}/batches/extend
```

- **Auth**: `authMwr` (shared `PaymentAPIToken`, same as `CountBatches`)
- **Request body**: `{"expected_last_self_extension_at": <RFC3339 | null>, "new_limit": int}`
- **Success**: `200 OK`, empty `{}`

Error responses carry `errorCode` on every error. No `Retry-After` — rate-limiting is subs's
concern, not skus's.

| Wire `errorCode` | HTTP status | Notes |
|---|---|---|
| `malformed_body` | 400 | |
| `order_not_found` | 404 | Includes `ErrInvalidOrderNoItems` and `ErrOrderItemNotFound` |
| `order_not_paid` | 402 | |
| `unsupported_cred_type` | 400 | Item is not TLV2 |
| `extension_conflict` | 409 | CAS lost — caller must refetch and retry |
| `extension_invalid_limit` | 422 | `new_limit` ≤ 0 or > 1000, or DB CHECK rejected |
| (none) | 499 | `context.Canceled` |
| (none) | 504 | `context.DeadlineExceeded` |
| (none) | 500 | Unexpected internal error |

---

## subscriptions public API

### Policy values (`pkg/api/subscription.go`)

```go
type extensionPolicy struct {
    SlotsPerGrant      int
    MinIntervalSeconds int
    MaxPerItem         int
}

var defaultExtensionPolicy = extensionPolicy{
    SlotsPerGrant:      3,
    MinIntervalSeconds: 30 * 24 * 60 * 60,
    MaxPerItem:         5,
}

var extensionPolicies = map[string]extensionPolicy{} // per-product overrides
```

`policyForProduct(p)` returns the per-product override if present, else `defaultExtensionPolicy`.
The map is empty today — every TLV2 product gets the default ceiling of 10 + 5×3 = 25.

### Service method (`pkg/api/subscription.go`)

```go
func (s *Server) extendLinkingLimit(ctx context.Context, subxID, subID uuid.UUID) error
```

`extendLinkingLimit` flow:

```
1. repositoryModule.getSubBySubxID(...)     ← ownership check
2. sub.OrderID.Valid                         → errSubNotRecognised
3. prodSet.ByID(sub.ProductID).IsTLV2()      → errSKUsUnsupportedCredType
4. orderModule.FetchOrder(sub.OrderID)
5. len(order.Items) == 1                     → errSKUsExtensionBundleOrder
6. pol = policyForProduct(p)
7. applyExtensionWithRetry(orderID, itemID, pol)
```

`applyExtensionWithRetry` loop (max 2 attempts):

```
a. orderModule.CountBatches(orderID)               → state {limit, active, num_self_extensions, last_self_extension_at}
b. evaluateExtensionPolicy(state, pol, now()):
       active < limit                  → errExtensionNotAtLimit
       num_self_extensions >= MaxPerItem → errExtensionMaxPerItem
       last_self_extension_at + MinInterval > now → errExtensionRateLimited
       otherwise → ExtensionWrite{ExpectedLastSelfExtensionAt: state.LastSelfExtensionAt, NewLimit: state.Limit + SlotsPerGrant}
c. orderModule.ExtendLinkingLimit(orderID, itemID, write)
       success                                        → return nil
       errSKUsExtensionConflict and attempts left     → loop with fresh state
       errSKUsExtensionConflict and exhausted         → return errSKUsExtensionConflict
       any other error                                → return as-is
```

Re-evaluating policy on retry is intentional: if a concurrent write changed state (e.g. the
user just did extend in another tab), the retry should re-check the rate-limit/max-per-item
gates against the fresh values rather than blindly trying again.

The read path (`GET .../credentials/batches/count`) returns `can_extend` computed by the
exact same gates — so a client that sees `can_extend: true` and POSTs immediately should
succeed unless someone else's write landed first (CAS retry handles that case).

### Client (`pkg/api/order.go`)

`orderModule.ExtendLinkingLimit` POSTs the CAS write to skus. Wire-code → typed-error mapping:

| skus wire code | subs Go error |
|---|---|
| `extension_conflict` | `errSKUsExtensionConflict` |
| `extension_invalid_limit` | `errSKUsExtensionInvalidLimit` |
| `malformed_body` | `errSKUsExtensionInvalidLimit` (caller bug — same surface as invalid limit) |
| `order_not_found` | `errSKUsOrderNotFound` |
| `order_not_paid` | `errSKUsOrderNotPaid` |
| `unsupported_cred_type` | `errSKUsUnsupportedCredType` |
| (missing / unrecognised) | fallback by HTTP status (404 / 402 / 409 / 422 / generic) |

Plus, before the wire-code switch:

- HTTP 499 (`StatusClientClosedConn`) → `model.ErrClientClosedConn`
- HTTP 504 (`StatusGatewayTimeout`) → `errSKUsDeadlineExceeded`

There is no `Retry-After` parsing — there is no rate-limit code from skus to parse.

### HTTP endpoints

```
POST /v1/subscriptions/{subscriptionID}/credentials/batches/extend
```

- **Auth**: subscriber JWT (`authContextKey` → `authClaims.SubscriberID`)
- **Request body**: none (subs owns policy, evaluates internally before calling skus)
- **Success**: `200 OK`, empty `{}`

Error mapping for the POST (subs-side, what the browser sees) — distinct codes per failure mode:

| subs Go error | HTTP status | Wire `errorCode` |
|---|---|---|
| `errSubNotRecognised`, `errSubscriptionNotFound`, `errSKUsOrderNotFound` | 404 | (none — collapses "not yours" and "doesn't exist" as a BOLA guard) |
| `errSKUsOrderNotPaid` | 402 | `order_not_paid` |
| `errSKUsUnsupportedCredType` | 400 | `unsupported_cred_type` |
| `errExtensionRateLimited` | 429 | `rate_limited` |
| `errExtensionMaxPerItem` | 422 | `max_per_item` |
| `errExtensionNotAtLimit` | 422 | `not_at_limit` |
| `errSKUsExtensionInvalidLimit` | 422 | `invalid_limit` |
| `errSKUsExtensionConflict` (after retry) | 409 | `conflict` |
| `errSKUsExtensionBundleOrder` | 422 | `bundle_order` |
| `ErrClientClosedConn` | 499 | (none) |
| `context.DeadlineExceeded`, `errSKUsDeadlineExceeded` | 504 | (none) |
| (default) | 500 | (none — only unexpected errors are logged at error level) |

Every gate-rejection is logged at info level (`"extension request rejected"`) with `subx_id` /
`sub_id` so operations can spot hot-itemID conflict storms or unusual rate-limit trips.

### Metrics

Server emits a single Prometheus counter per request:

```
linking_limit_extension_outcome{outcome="<label>"}
```

Labels (one per terminal path): `granted`, `conflict_retried`, `conflict_persisted`,
`rate_limited`, `max_per_item`, `not_at_limit`, `invalid_limit`, `bundle_order`,
`order_not_found`, `order_not_paid`, `unsupported_cred_type`, `internal_error`.
`conflict_retried` is the success-after-CAS-conflict signal: a sustained rise indicates a
hot itemID under concurrent self-extend pressure. `conflict_persisted` indicates the retry
also lost — investigate.

The plan is to keep distinct codes during rollout and pare overlap (e.g. fold
`bundle_order` / `not_at_limit` / `invalid_limit` if clients don't differentiate) once we have
real client telemetry.

### Router registration (`pkg/api/subscription.go`)

```go
r.HandleFunc("/{subscriptionID}/credentials/batches/extend",
    handlers.AppHandler(h.canExtend).ServeHTTP).
    Methods(http.MethodGet).Name("CanExtend")

r.HandleFunc("/{subscriptionID}/credentials/batches/extend",
    handlers.AppHandler(h.extendLinkingLimit).ServeHTTP).
    Methods(http.MethodOptions, http.MethodPost).Name("ExtendLinkingLimit")
```

Two registrations on the same path: GET answers `can_extend`, POST does the extension. The
POST registration declares `MethodOptions` so gorilla/mux serves the browser's CSRF
preflight from there — the GET doesn't need it (browsers don't preflight simple GETs).

---

## Status endpoint changes

skus exposes the raw tracking fields only on the server-to-server endpoint used by subs.
The order JSON keeps `num_self_extensions` and `last_self_extension_at` as `json:"-"` —
those fields no longer leak through `GET /v1/orders/{orderID}`.

### skus internal endpoint

`GET /v1/orders/{orderID}/credentials/batches/count` — full state for the policy decision:

```json
{
  "limit": 10,
  "active": 7,
  "num_self_extensions": 2,
  "last_self_extension_at": "2026-03-15T12:34:56Z"
}
```

Auth'd with `PaymentAPIToken`, not browser-callable.

### subs public endpoint

subs reshapes the response to a single `can_extend` boolean. No counts, no timestamps, no
raw counters reach the client — every policy decision is collapsed into this one field.

`GET /v1/subscriptions/{subscriptionID}/credentials/batches/extend`:

```json
{
  "can_extend": false
}
```

GET and POST share the same `/credentials/batches/extend` path: GET answers "may I extend
right now?", POST does the extension. The old `/credentials/batches/count` route was
dropped — it was a misnamed leftover once `limit`/`active` left the response.

`can_extend` is true iff *all three* policy gates pass right now:

- `active >= limit` (at-limit; no point extending if you have free slots)
- `num_self_extensions < MaxPerItem` (haven't burned the lifetime cap)
- `now >= last_self_extension_at + MinIntervalSeconds` (rate-limit window has elapsed, or no
  prior extension)

Computed by `canExtend(state, pol, now)` in subs — exactly the same gates that
`evaluateExtensionPolicy` enforces on the write path, so a `can_extend: true` response is a
strong signal that the corresponding POST will succeed (modulo concurrent writes from another
tab, which the CAS retry handles).

`limit` and `active` were dropped from the response per product feedback — the client
shouldn't branch on those values directly. With those fields gone the route was renamed
from `.../batches/count` (a now-misleading name) to GET `.../batches/extend` so the read
and the write share a path: same nouns, different verbs.

The lightweight skus `extension-state` endpoint we briefly added was also dropped: with
`can_extend` requiring the active-batches aggregate (for the at-limit gate), there is no
cheaper read worth splitting out.

---

## Compatibility

This is a coordinated breaking change between bat-go (skus) and subscriptions. The skus wire
contract is incompatible with the prior version (different request body, different error codes,
no `Retry-After`). The two services must ship together — there is no v1/v2 transition.

---

## Tests

### bat-go primitive

- **Handler** (`handler/cred_test.go`, `TestCred_ExtendLinkingLimit`): table-driven cases for
  `invalid_orderID`, `invalid_itemID`, `malformed_body`, `context_canceled`, `deadline_exceeded`,
  `order_not_found`, `order_not_paid`, `unsupported_cred_type`, `extension_invalid_limit`,
  `extension_conflict`, `internal_error`, `success`. Every error case asserts the wire `errorCode`.

- **Service** (`service_nonint_test.go`, `TestService_ExtendLinkingLimit`): one case per
  defensive guard plus CAS-success / CAS-conflict / CHECK-violation, using `MockOrderItem`
  with `FnApplyExtensionCAS`.

- **Repository integration** (`storage/repository/order_item_test.go`,
  `TestOrderItem_ApplyExtensionCAS`, build-tagged `integration`): hits a real Postgres to
  verify (a) first extension succeeds with nil token, (b) stale token returns
  `ErrExtensionConflict`, (c) matching token after a prior extension succeeds, (d) `new_limit
  > 1000` triggers the CHECK constraint and returns a `pq.Error` code `23514`.

### subscriptions public API

- **Handler** (`handler_subscription_test.go`, `TestSubHandler_extendLinkingLimit`): every row
  of the error-mapping table above against a mocked `subSvc`. Asserts wire `errorCode`,
  HTTP status, and cause for each.

- **Service** (`subscription_test.go`, `TestServer_extendLinkingLimit`): each early-exit
  guard, plus the new flow: count-batches error bubbles, policy gates fire (not-at-limit /
  max-per-item / rate-limit-window), happy path freezes the CAS contract by asserting the
  write's `ExpectedLastSelfExtensionAt` and `NewLimit`, conflict-then-success-on-retry,
  conflict-persists-after-retry.

- **Policy unit** (`subscription_test.go`, `TestEvaluateExtensionPolicy`): table-driven for
  each branch (not-at-limit, max-per-item, rate-limit window, first-extension-no-token,
  subsequent-extension-passes-token).

- **canExtend helper** (`subscription_test.go`, `TestCanExtend`): each gate (not-at-limit,
  max-per-item, rate-limit window) returns false; happy paths (at-limit-and-never-extended,
  window-elapsed-and-under-cap) return true.

- **Client** (`order_test.go`, `TestOrderModule_ExtendLinkingLimit`): every wire `errorCode`
  → typed-error mapping (`extension_conflict`, `extension_invalid_limit`, `malformed_body`,
  `order_not_found`, `order_not_paid`, `unsupported_cred_type`), `504` →
  `errSKUsDeadlineExceeded`, `499` → `model.ErrClientClosedConn`, status fallback paths
  (404 / 402 / 409 / 422 / generic), and a happy path that asserts URL, method, auth header,
  and serialised request body shape (`new_limit`, `expected_last_self_extension_at`).

---

## Out of scope (flagged during spec review)

Tracked here so they don't get lost; each needs a separate discussion with product/support:

- **`access_forbidden` wire code**: product spec names this; we currently return 404 for both
  "not yours" and "doesn't exist" as a BOLA guard. Options: keep 404 + set
  `errorCode: "access_forbidden"` on it, or split 403/404. Needs product input.
- **Support revocation of extension ability**: spec calls for a way to disable self-service
  extension for a given user without touching `max_active_batches_tlv2_creds`. No skus
  endpoint or CLI today; ops would need direct DB access to clear or bump
  `num_self_extensions` / `last_self_extension_at`. Proper fix would be either a dedicated
  skus support endpoint that mutates the tracking columns, or a schema flag
  (e.g. `self_extensions_revoked_at timestamptz NULL`). Both are product-scope decisions —
  flagged here so it's not lost.
- **Client-side UI** — browser detects the limit error on credential fetch, surfaces the
  extension option, calls the subscriptions endpoint, and retries. Separate workstream.
- **Purchase flow (vector 3)** — separate workstream. The DB columns and endpoint design
  accommodate it when the time comes.
- **Support CLI** — tooling changes live in `tools/skus/cmd/skus.go` on this branch; coverage
  against the product spec (manual increment / reset) to be verified with support.
- **Distinct vs collapsed error codes** — current mapping keeps `not_at_limit`,
  `max_per_item`, `bundle_order`, `invalid_limit` as separate 422s and `rate_limited` /
  `conflict` as their own statuses. Once we have real client telemetry, fold any that clients
  don't differentiate.
