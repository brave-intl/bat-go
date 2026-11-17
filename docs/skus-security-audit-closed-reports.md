# Known out-of-scope security reports for services/skus

The issues below have been reported, re-verified against this codebase, and closed as out of scope. They are documented here so reviewers and scanners have the disposition on record, along with the defenses that make each issue out of scope and why the claimed impact does not hold.

## Re-report policy

Re-reports of the issues listed in this document will be closed as Not Applicable without bounty. Repeated re-reporting of a closed issue listed here may result in suspension of program access per HackerOne guidelines.

## Unauthenticated order and credential endpoints

**Known issue:** order-detail and credential routes intentionally carry no authentication middleware.

**Defenses:** order identifiers are random UUIDv4 values, so another user's order cannot be enumerated; credential issuance is gated on the order's paid status and expiry (`service.go:1882`).

**Why the impact is invalid:** reaching another user's order or credentials requires already possessing that user's order identifier, which the described adversary has no mechanism to obtain.

## Cross-merchant order access

**Known issue:** some merchant-authenticated routes skip the per-object merchant-and-caveat validation.

**Defenses:** every such route still requires a valid merchant HTTP-signature key (`key.go:186`); merchant keys are provisioned only to Brave and its merchants.

**Why the impact is invalid:** there is no path from an unauthenticated client or an ordinary authenticated client; crossing to another merchant's orders requires a provisioned signing key.

## Zero-price order creation

**Known issue:** the item price on the authenticated order-creation endpoint is not schema-validated, so zero or negative prices could produce zero-value orders.

**Defenses:** the route requires either the internal simple token or a valid merchant HTTP signature (`key.go:186`).

**Why the impact is invalid:** only holders of those credentials can reach the endpoint; they are not available to internet clients.

## Device-cap race on time-limited-v2 credentials

**Known issue:** concurrent credential-batch requests can exceed the per-order device cap.

**Defenses:** the device cap is a policy limit, not the payment gate; credential issuance remains gated on the order being paid.

**Why the impact is invalid:** no order is marked paid without payment, so the race does not yield premium value without paying.

## Refund without entitlement revocation

**Known issue:** an App Store REFUND notification cancels the order without revoking an already-issued perpetual license.

**Defenses:** the entitlement originated from a completed purchase.

**Why the impact is invalid:** this is a revocation-lifecycle question, not a payment-gate bypass; the underlying purchase was paid.

## Hardcoded credentials

**Known issue:** a claim that authentication secrets (`TOKEN_LIST`, `MACAROON_SECRET`) are hardcoded in this repository.

**Defenses:** neither value is hardcoded in the source; both are supplied through environment configuration, and the SKU whitelist is built from environment-injected SKUs (`skus.go:112`).

**Why the impact is invalid:** no credential exists in the source to extract, and deployment configuration is outside the bounty scope for code findings.
