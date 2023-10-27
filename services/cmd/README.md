Below is the command structure for bat-go microservices using cobra

## create a macaroon
```bash
# needs an example.yaml file to work
./bat-go macaroon create
```
with options
```bash
MACAROON_SECRET=a9ed2c16-3cf6-446f-8978-e707bead3979 \
./bat-go macaroon create --config "macaroon-config.yaml"
```

## start rewards rest server
```bash
./bat-go serve rewards rest
```
with options
```bash
./bat-go serve rewards rest \
  --config "config.yaml" \
  --ratios-token "abc" --ratios-service "123" --environment "local" \
  --base-currency "USD" --address ":4321"
```

## start rewards grpc server
```bash
./bat-go serve rewards grpc
```
with options
```bash
./bat-go serve rewards grpc \
  --config "config.yaml" \
  --ratios-token "abc" --ratios-service "123" --environment "local" \
  --base-currency "USD" --address ":4321"
```

## check server fingerprints
```bash
./bat-go get-cert-fingerprint "brave.com:443"
```

## paypal settlement

### transform
```bash
./bat-go settlement paypal transform \
  --input "paypal-settlement-from-antifraud.json" \
  --currency "JPY"
```

with other options
```bash
./bat-go settlement paypal transform \
  --input "paypal-settlement-from-antifraud.json" \
  --currency "JPY" \
  --rate "104.75" \
  --out "paypal-settlement-from-antifraud-complete.json"
```

### complete
```bash
./bat-go settlement paypal complete \
  --input "paypal-settlement-from-antifraud.json" \
  --txn-id "30ad1991-b2d3-4897-ae52-09efcd174235"
```

with output option
```bash
./bat-go settlement paypal complete \
  --input "paypal-settlement-from-antifraud.json" \
  --txn-id "30ad1991-b2d3-4897-ae52-09efcd174235" \
  --out   "paypal-settlement-complete.json"
```

### email
```bash
./bat-go settlement paypal email \
  --input "paypal-settlement-from-antifraud.json"
```

## gemini settlement

### upload

```bash
./bat-go settlement gemini upload \
  --input "gemini-contribution-signed.json" \
  --all-txs-input "4ae996f5-679b-46c6-9aea-9892f763ffe6" \
  --sig 0
```

with output
```bash
./bat-go settlement gemini upload \
  --input "gemini-contribution-signed.json" \
  --all-txs-input "4ae996f5-679b-46c6-9aea-9892f763ffe6" \
  --sig 0 \
  --out "gemini-contribution-signed-completed.json"
```

### checkstatus

```bash
./bat-go settlement gemini checkstatus \
  --input "gemini-contribution-signed.json" \
  --all-txs-input "4ae996f5-679b-46c6-9aea-9892f763ffe6"
```

## bitflyer settlement

equivalent envs are available as flags `ENV_KEY` -> `--env-key`

### refresh token
After running the refres token command, you will need to copy the value in the printed `auth.access_token` field into your `.env` file and source that file. This can now be used with the other bitflyer commands. The env name should be `BITFLYER_TOKEN`.
```bash
BITFLYER_CLIENT_ID=
BITFLYER_CLIENT_SECRET=
BITFLYER_EXTRA_CLIENT_SECRET=
BITFLYER_SERVER=
./bat-go settlement bitflyer token
```

at this point, it makes sense to run the `sign-settlement` command so that transactions are split across multiple files, however this is not strictly necessary to do because we do all of the transforms needed in the upload step.

### upload

```bash
BITFLYER_SOURCE_FROM=tipping
BITFLYER_SERVER=
# omit to execute
BITFLYER_DRYRUN=1 # seconds to delay
./bat-go bitflyer upload \
  --in "bitflyer-transactions.json" \
  --exclude-limited true # if a transaction ever hits transfer limit, do not send it
```

### checkstatus

```bash
BITFLYER_SOURCE_FROM=tipping
BITFLYER_SERVER=
./bat-go bitflyer checkstatus \
  --in "bitflyer-transactions.json"
```

## wallet

### create

```bash
./bat-go wallet create \
  --provider "uphold" \
  --name "test"
```

### vault create wallet
```bash
./bat-go vault create-wallet
```
create offline
```bash
./bat-go vault create-wallet --offline true
```

### transfer funds

with vault inputs
```bash
./bat-go wallet transfer-funds \
  --provider "uphold" \
  --from "1234567890" \
  --to "1234567890" \
  --usevault true \ # get secrets from vault
  --value "10.5"
```

with env inputs
```bash
ED25519_PRIVATE_KEY= \
UPHOLD_PROVIDER_ID= \
./bat-go wallet transfer-funds \
  --provider "uphold" \
  --from "1234567890" \
  --to "1234567890" \
  --value "10.5"
```

## vault

### init
```bash
./bat-go vault init \
  --key-shares 1 \
  --key-threshold 1 \
  ./key.asc
```

### import key

```bash
ED25519_PRIVATE_KEY=
ED25519_PUBLIC_KEY=
UPHOLD_PROVIDER_ID=
GEMINI_CLIENT_ID=
GEMINI_CLIENT_KEY=
GEMINI_CLIENT_SECRET=
./bat-go vault import-key \
  --config "config.yaml"
```

only import a subset of the keys with `--wallet-refs`
```bash
./bat-go vault import-key \
  --config "config.yaml" \
  --wallet-refs "gemini-referral"
```

### sign settlement

uses inputs from vault
```bash
./bat-go vault sign-settlement \
  --config "config.yaml" \
  --input "contributions.json" \
  --providers "uphold,gemini"
```

### unseal

unseals vault
```bash
gpg -d ./share-0.gpg | ./bat-go vault unseal
```

## generate

### json-schema

generates json schemas
```bash
go run main.go generate json-schema
```
with override
```bash
go run main.go generate json-schema --overwrite
```
