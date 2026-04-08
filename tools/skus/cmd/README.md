# bat-go skus — Support Runbook

## Building

The `bat-go` binary is built from the `main/` module at the root of the repository, which pulls in the skus tooling via its import of `tools/skus/cmd`.

```bash
cd main
go build -o bat-go .
```

This produces a `bat-go` binary in the `main/` directory. Move it somewhere on your `PATH` or invoke it with its full path.

You need Go 1.25 or later. Run `go version` to check.

---

## reset-linking-limit

Frees device linking slots for a premium subscriber. When a user hits their device limit and can't link new devices, this command deletes the oldest active credential batches (one batch = one linked device slot).

### Prerequisites

**Binary**: `bat-go` must be built and in your PATH.

**Private key**: an ed25519 key in SSH format, authorized for the target environment. Obtain from the ops team or retrieve from the environment's sealed secrets.

**Support API token**: required only when looking up by email. Obtain from the ops team (`SUPPORT_API_TOKEN` from the subscriptions service secrets for the target environment).

### Flags

| Flag | Env var | Required | Description |
|------|---------|----------|-------------|
| `--skus-base-url` | `SKUS_BASE_URL` | Yes | Base URL of the SKUs/payments service |
| `--private-key` | `SKUS_SUPPORT_PRIVATE_KEY` | Yes | Path to your ed25519 private key file |
| `--seats` | — | Yes | Number of device slots to free |
| `--order-id` | — | One of these | Order UUID (mutually exclusive with `--email`) |
| `--email` | `SUBSCRIBER_EMAIL` | One of these | Subscriber email (mutually exclusive with `--order-id`) |
| `--subscriptions-base-url` | `SUBSCRIPTIONS_BASE_URL` | Yes, with `--email` | Base URL of the subscriptions service |
| `--subscriptions-token` | `SUBSCRIPTIONS_SUPPORT_TOKEN` | Yes, with `--email` | Bearer token for the support API |
| `--item-id` | — | No | Scope the reset to a specific order item UUID |

Flags take precedence over env vars.

### Usage

#### When you have the order ID

```bash
bat-go skus reset-linking-limit \
  --skus-base-url https://payment.rewards.brave.com \
  --order-id <ORDER_UUID> \
  --seats <N> \
  --private-key /path/to/operator.key
```

#### When the user provides only their email

```bash
bat-go skus reset-linking-limit \
  --skus-base-url https://payment.rewards.brave.com \
  --subscriptions-base-url https://subscriptions.rewards.brave.com \
  --subscriptions-token <SUPPORT_API_TOKEN> \
  --email <USER_EMAIL> \
  --seats <N> \
  --private-key /path/to/operator.key
```

If the email matches multiple active subscriptions (e.g. VPN + Leo), the command prints a numbered list and asks you to select one before proceeding.

### Interactive flow

The command always shows what it will do before making changes:

```
Order bf399efe-... has 3 active device batch(es).

Oldest 1 batch(es) at time of listing:

  request_id                                oldest_valid_from (UTC)
  ----------------------------------------  ------------------------
  7a1c3e2d-...                              2025-11-01T00:00:00Z

Note: the server selects the oldest N batches independently at delete time.
      If the order changes before the request arrives, the result may differ.

Delete 1 seat(s) for order bf399efe-...? [y/N]:
```

Type `y` to confirm. Anything else aborts with no changes made.

### How many seats to free

`--seats` is the number of device slots to release. Each seat = one linked device removed (oldest first). If the user wants to add one new device, free 1 seat. If you're unsure, start with 1 — they can always ask again.

If `--seats` exceeds the number of active batches, the command warns you and caps at the actual count. You will not accidentally delete more than exists.

### Environment reference

| Environment | `--skus-base-url` |
|-------------|-------------------|
| Staging | `https://grant.rewards.brave.software` |
| Production | `https://payment.rewards.brave.com` |

The subscriptions service URL and support token are environment-specific — confirm with the ops team.

### Troubleshooting

**"no subscriber found for email"** — the email is not in the subscriptions database. Ask the user to confirm the email address on their Brave account.

**"no active subscriptions found for email"** — the subscriber exists but has no currently active subscription (expired or never had one).

**"No active device batches found for this order"** — the order has no linked devices to clear. The user's limit issue may have a different cause.

**Unexpected status 401** — your private key is not in the authorized keystore for this environment, or your system clock is off by more than 10 minutes (requests are signed with a timestamp).
