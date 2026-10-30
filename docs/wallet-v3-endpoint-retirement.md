# Retiring the unauthenticated v3 wallet GET endpoints

The unauthenticated `GET` routes on `/v3/wallet` are repeatedly reported on HackerOne as lacking authentication. This document records the disposition for reviewers, scanners, and re-reporters: the reported behavior is not a concern, and the routes are additionally being retired because authenticated v4 replacements fully cover them.

## Routes removed

| Removed route (unauthenticated) | Replacement (HTTP-signed) |
| --- | --- |
| `GET /v3/wallet/{paymentID}` | `GET /v4/wallets/{paymentID}` |
| `GET /v3/wallet/uphold/{paymentID}` | `GET /v4/wallets/uphold/{paymentID}` |
| `GET /v3/wallet/recover/{publicKey}` | none — see "Recovery by public key" below |

The route registrations and handlers were removed from `services/wallet/service.go` and `services/wallet/controllers_v3.go`; the uphold-balance logic now lives directly in `GetUpholdWalletBalanceV4` (`services/wallet/controllers_v4.go`) rather than forwarding to the v3 symbol.

## Unauthenticated wallet record lookup

**Known issue:** the wallet-detail and uphold-balance routes return wallet records without authentication middleware.

**Defenses:** wallet identifiers are random UUIDv4 values, so another user's wallet cannot be enumerated; every route that can move or modify funds — wallet creation, custodian claim/connect, the v4 create/update/get routes — requires an HTTP signature derived from the client's recovery-seed key (`middleware.HTTPSignedOnly`) or the internal simple token.

**Why the impact is invalid:** reaching a wallet record requires already possessing that wallet's payment id, and the record contains no credential that grants spend or transfer capability — the public key and provider linking metadata in the response are identifier-bound data by design, and possessing them enables nothing without the matching signing key. The described adversary learns nothing they can act on.

## Recovery by public key

**Known issue:** `GET /v3/wallet/recover/{publicKey}` resolved a wallet record from a public key without authentication — effectively a key-to-payment-id lookup.

**Defenses:** a public key is public by definition, and the response is the wallet record bound to that key; it contains no secret material and no capability without the matching private key.

**Why the impact is invalid:** the requester must already hold the public key, and the response is derivable from it; nothing is disclosed that enables impersonation or spending.

**Why it is still removed:** no shipping client calls it — brave clients persist `payment_id` together with the recovery seed (`components/brave_rewards/core/engine/wallet/wallet.cc:104`) and derive signing keys locally from the seed (`components/brave_rewards/core/engine/util/request_signer.cc:57`), a wallet with a seed but no payment id is treated as unusable (`rewards_engine.cc:417`), and there are zero references to the route in brave-core or brave-ios. Removing it retires the report magnet; if legacy-client traffic ever appears, the route can be reintroduced behind the v4 signature middleware rather than unauthenticated.

## Remaining v3 surface (intentionally kept)

The rest of the `/v3/wallet` surface stays and is not affected by the reports: `POST /v3/wallet/brave` (creation fallback when geo country is unavailable), custodian claim/connect routes, `GET /v3/wallet/linking-info` (simple-token protected), `POST /v3/wallet/challenges` (Solana dApp linking), and the Solana waitlist routes. These are used by shipping clients and several require HTTP signatures already.

## Re-report policy

Re-reports of the removed routes listed in this document will be closed as Not Applicable without bounty. Repeated re-reporting of a closed issue listed here may result in suspension of program access per HackerOne guidelines.
