# Retiring the unauthenticated v3 wallet GET endpoints

The unauthenticated `GET` routes on `/v3/wallet` are repeatedly reported on HackerOne as lacking authentication. This document records the disposition: those routes are removed rather than documented as out of scope, because authenticated v4 replacements fully cover them. It is kept here so reviewers, scanners, and re-reporters have the decision and the client evidence on record.

## Routes removed

| Removed route (unauthenticated) | Replacement (HTTP-signed) |
| --- | --- |
| `GET /v3/wallet/{paymentID}` | `GET /v4/wallets/{paymentID}` |
| `GET /v3/wallet/uphold/{paymentID}` | `GET /v4/wallets/uphold/{paymentID}` |
| `GET /v3/wallet/recover/{publicKey}` | none — see "Recovery endpoint" below |

The route registrations and handlers were removed from `services/wallet/service.go` and `services/wallet/controllers_v3.go`; the uphold-balance logic now lives directly in `GetUpholdWalletBalanceV4` (`services/wallet/controllers_v4.go`) rather than forwarding to the v3 symbol.

## Why removal is safe

- **Response superset:** `ResponseV4` (`services/wallet/outputs.go:157`) contains every `ResponseV3` field plus `selfCustodyAvailable`, built from the same underlying wallet record, so the v4 GET returns strictly more data for the signed client.
- **Identical balance logic:** `GetUpholdWalletBalanceV4` previously forwarded verbatim to the v3 handler; the inlined body is unchanged.
- **No shipping client calls the v3 GETs:** brave-core references only `/v4/wallets/{payment_id}` for wallet lookup (`components/brave_rewards/core/engine/endpoints/brave/get_wallet.cc`), and the same is true for iOS. There are zero references to the three v3 GET routes in brave-core or brave-ios.

## Recovery endpoint

`GET /v3/wallet/recover/{publicKey}` returned a wallet record for any public key without authentication — effectively a key-to-payment-id lookup, which is the property the reports flag. No shipping client needs it: a brave client persists its `payment_id` together with its recovery seed (`components/brave_rewards/core/engine/wallet/wallet.cc:104`) and derives signing keys locally from the seed (`components/brave_rewards/core/engine/util/request_signer.cc:57`); a wallet with a seed but no payment id is treated as unusable (`rewards_engine.cc:417`). A client therefore never needs to ask the server for its own payment id.

Infra traffic counts are the gate for this route: if legacy-client usage appears, the route can be reintroduced behind the v4 signature middleware (`HTTPSignedOnly`) rather than resurrected unauthenticated.

## Remaining v3 surface (intentionally kept)

The rest of the `/v3/wallet` surface stays and is not affected by the reports: `POST /v3/wallet/brave` (creation fallback when geo country is unavailable), custodian claim/connect routes, `GET /v3/wallet/linking-info` (simple-token protected), `POST /v3/wallet/challenges` (Solana dApp linking), and the Solana waitlist routes. These are used by shipping clients and several require HTTP signatures already.

## Re-report policy

Re-reports of the removed routes listed in this document will be closed as Not Applicable without bounty. Repeated re-reporting of a closed issue listed here may result in suspension of program access per HackerOne guidelines.
