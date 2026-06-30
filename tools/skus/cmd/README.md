# bat-go skus — Support Runbook

## Building

The `bat-go` binary is built from the `main/` module at the root of the repository, which pulls in the skus tooling via its import of `tools/skus/cmd`.

```bash
cd main
go build -o bat-go .
```

This produces a `bat-go` binary in the `main/` directory. Move it somewhere on your `PATH` or invoke it with its full path.

You need Go 1.26 or later. Run `go version` to check.

---

## Overview

All commands talk to the subscriptions service support API. Each request is signed with your ed25519 operator key; the public half must be registered in the subscriptions support keystore for the target environment. There is no direct SKUs access.

| Command | What it does |
|---------|--------------|
| `show-linking-usage` | Read-only: shows device slots in use out of the limit, one row per linked device |
| `reset-linking-limit` | Frees N device slots by deleting the oldest credential batches |
| `extend-linking-limit` | Grants a policy-gated linking-limit extension (same gates as the in-browser self-service flow) |

### Prerequisites

**Binary**: `bat-go` must be built and in your PATH.

**Operator key**: an ed25519 private key (SSH format) whose public key has been added to the subscriptions support keystore for the target environment. Generate one with `ssh-keygen -t ed25519 -f ~/.ssh/brave_support` and send the `.pub` to the ops team to register.

### Shared flags

Every command takes the same connection and identification flags:

| Flag | Env var | Required | Description |
|------|---------|----------|-------------|
| `--subscriptions-base-url` | `SUBSCRIPTIONS_BASE_URL` | Yes | Base URL of the subscriptions service |
| `--private-key` | `SUBSCRIPTIONS_SUPPORT_PRIVATE_KEY` | Yes | Path to your ed25519 private key file (SSH format) used to sign requests |
| `--subscription-id` | — | One of these | Subscription UUID (mutually exclusive with `--email`) |
| `--email` | `SUBSCRIBER_EMAIL` | One of these | Subscriber email (mutually exclusive with `--subscription-id`) |

Flags take precedence over env vars.

If the email matches multiple active subscriptions (e.g. VPN + Leo), the command prints a numbered list and asks you to select one before proceeding.

---

## show-linking-usage

Read-only check of a subscriber's device slot usage. Run this first when a user reports a linking problem.

```bash
bat-go skus show-linking-usage \
  --subscriptions-base-url https://subscriptions.rewards.brave.com \
  --private-key ~/.ssh/brave_support \
  --email <USER_EMAIL>
```

```
3 of 10 device slot(s) in use for subscription 1f14a340-... (vpn).

  request_id                                oldest_valid_from (UTC)
  ----------------------------------------  ------------------------
  7a1c3e2d-...                              2025-11-01T00:00:00Z
  ...
```

---

## reset-linking-limit

Frees device linking slots for a premium subscriber. When a user hits their device limit and can't link new devices, this command deletes the oldest active credential batches (one batch = one linked device slot).

Additional flag:

| Flag | Required | Description |
|------|----------|-------------|
| `--slots` | Yes | Number of device-linking slots to free |

```bash
bat-go skus reset-linking-limit \
  --subscriptions-base-url https://subscriptions.rewards.brave.com \
  --private-key ~/.ssh/brave_support \
  --email <USER_EMAIL> \
  --slots <N>
```

### Interactive flow

The command always shows the current usage before making changes:

```
3 of 10 device slot(s) in use for subscription 1f14a340-... (vpn).

  request_id                                oldest_valid_from (UTC)
  ----------------------------------------  ------------------------
  7a1c3e2d-...                              2025-11-01T00:00:00Z
  ...

Note: the server selects the oldest N batches independently at delete time.
      If the subscription changes before the request arrives, the result may differ.

Delete 1 slot(s) for subscription 1f14a340-...? [y/N]:
```

Type `y` to confirm. Anything else aborts with no changes made.

### How many slots to free

`--slots` is the number of device-linking slots to release. Each slot = one linked device removed. If the user wants to add one new device, free 1 slot. If you're unsure, start with 1 — they can always ask again.

If `--slots` exceeds the number of slots in use, the command errors and makes no change. You will not accidentally delete more than exists.

---

## extend-linking-limit

Grants a self-service-style linking-limit extension, adding device slots without unlinking a device. Subscriptions enforces the same policy as the in-browser self-service flow: the subscriber must be at their device limit, under the lifetime extension cap, and outside the rate-limit window. Ineligible subscribers get the reason and no change is made.

```bash
bat-go skus extend-linking-limit \
  --subscriptions-base-url https://subscriptions.rewards.brave.com \
  --private-key ~/.ssh/brave_support \
  --email <USER_EMAIL>
```

---

## Environment reference

The subscriptions service URL is environment-specific, and your operator public key must be registered in that environment's support keystore — confirm with the ops team.

## Troubleshooting

**"no subscriber found for email"** — the email is not in the subscriptions database. Ask the user to confirm the email address on their Brave account.

**"no active subscriptions found for email"** — the subscriber exists but has no currently active subscription (expired or never had one).

**"--slots (N) exceeds device slots in use (M)"** — the user has fewer linked devices than you asked to free. Re-run with a smaller `--slots` (or none may need freeing — check the usage output).

**"reset failed (status 422, slots_exceeded)"** — a device was unlinked between the usage check and your confirmation. Re-run the command to see fresh usage.

**"extension failed (status 422, not_at_limit)"** — the subscriber has free slots; an extension isn't needed. Use `show-linking-usage` to confirm.

**"extension failed (status 429, rate_limited)"** — an extension was granted recently; the per-window rate limit applies to support grants too.

**Unexpected status 401** — the request was not signed (missing `--private-key`).

**Unexpected status 403** — your key signed the request but its public key is not in this environment's support keystore (wrong env, or not yet registered). Confirm with the ops team.
