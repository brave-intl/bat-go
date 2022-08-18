# Table of Contents 
- [Creating new local vault instance](#creating-new-local-vault-instance)
- [Bringing up vault](#bringing-up-vault)
- [Creating a vault config](#creating-a-vault-config)
- [Importing keys](#importing-keys)
- [Running settlement](#running-settlement)
- [Creating a new offline wallet](#creating-a-new-offline-wallet)
- [Signing Files](#signing-files)
- [Uploading files](#uploading-files)

## Creating new local vault instance

Vault is an on-device secure key-value store. 

If you have trouble installing from the README, you can also 
(1) Download from https://www.vaultproject.io/downloads
(2) make download-vault and install using 4096 rather than ed25519

```
# add vault address into profile or similar for your shell
echo "export VAULT_ADDR=http://127.0.0.1:8200" >> ~/.profile

# set in local shell
export VAULT_ADDR=http://127.0.0.1:8200

# start vault server
./vault server -config=config.hcl

# initialize
./bat-go vault init --key-shares 1 --key-threshold 1 PATH_TO_GPG_PUB_KEY_FILE

# unseal vault
gpg -d ./share-0.gpg | ./bat-go vault unseal
# -> prompt for password
```

## Bringing up vault

On the offline computer, in one window run:
```
./vault server -config=config.hcl
```

In another run:
```
gpg -d ./share-0.gpg | ./bat-go vault unseal
```

## Creating a vault config
The `config.example.yaml` should be copied wherever it is easiest to point to. Just pass in the path while running a command that interacts with vault (import-key, sign-settlement) etc. Be sure to change the values of the wallets to suit your setup if required.

The values name should be unique, as we use the label online (e.g. creating a wallet with the label name).

As of May 2021, the following keys are accepted:

```bash
wallets:
  uphold-contribution: 'prod-wallet-contributions-YYYY-MM'
  uphold-referral: 'prod-wallet-referrals-YYYY-MM'
  gemini-contribution: 'prod-wallet-contributions-YYYY-MM'
  gemini-referral: 'prod-wallet-referrals-YYYY-MM'
```

## Importing keys

the following line imports the keys from the following environment variables:
```bash
ED25519_PRIVATE_KEY= \
ED25519_PUBLIC_KEY= \
UPHOLD_PROVIDER_ID= \
GEMINI_CLIENT_ID= \
GEMINI_CLIENT_KEY= \
GEMINI_CLIENT_SECRET= \
./bat-go vault import-key --config=./config.yaml
# pass a known key to only import one: --wallet-refs=gemini-referral
```

## Running settlement

First bring up vault as described above.

```
./bat-go vault sign-settlement --config=./config.yaml --in=SETTLEMENT_REPORT.JSON
```

Finally seal the vault:
```
./vault operator seal
```
You can now stop the server instance.

Copy the signed json flie to the usb stick and transfer it to the online
computer.

On the online computer:
```
export UPHOLD_ENVIRONMENT=
export UPHOLD_HTTP_PROXY=
export UPHOLD_ACCESS_TOKEN=
export UPHOLD_SETTLEMENT_ADDRESS=
export VAULT_ADDR=
./settlement-submit -in <SIGNED_SETTLEMENT.JSON>
```

Note that you can run submit multiple times, progress is tracked in a log to
allow restoring from errors and to avoid duplicate payouts.

Finally upload the "-finished" output file to eyeshade to account for payout
transactions that were made.

## Creating a new offline wallet

On the offline machine, first bring up vault as described above.

Run vault-create-wallet, this will sign the registration and store it into
a local file:
```
./bat-go vault create-wallet NAME_OF_NEW_WALLET
```

Copy the created `name-of-new-wallet-registration.json` file to the online
machine.

Re-run vault-create-wallet, this will submit the pre-signed registration:
### Uphold
```
export UPHOLD_ENVIRONMENT=
export UPHOLD_HTTP_PROXY=
export UPHOLD_ACCESS_TOKEN=
export VAULT_ADDR=
./bat-go vault create-wallet -offline NAME_OF_NEW_WALLET
```

### Gemini
```
export GEMINI_CLIENT_ID
export GEMINI_CLIENT_KEY
export GEMINI_CLIENT_SECRET
export GEMINI_SERVER
export GEMINI_SUBMIT_TYPE
export VAULT_ADDR
```

Finally copy `name-of-new-wallet-registration.json` back to the offline
machine and run vault-create-wallet to record the provider ID in vault:
```
./bat-go vault create-wallet -offline NAME_OF_NEW_WALLET
```

## Signing Files
Signing the settlement file will split the input files into many output files depending on the contents of the file

### Uphold
```bash
./bat-go vault sign-settlement --config=./config.yaml --in=contributions.json
```

### Gemini
```bash
./bat-go vault sign-settlement --config=publishers-gemini.yaml --in=publishers-payout-report-gemini-referrals.json --providers=gemini
```

## Uploading files

Running `settlement-submit` with a provider tells the script where to submit the file and the kind of handler to use.

### Uphold
```bash
./settlement-submit -in=gemini-contributions-signed.json -provider=uphold
```

### Gemini

gemini has a command available to it for uploading transactions and sending
```bash
./bat-go settlement gemini upload --input=gemini-referral-publishers-payout-report-gemini-referrals-signed.json --all-txs-input=publishers-payout-report-gemini-referrals.json
```

and to check the status of each transaction a `checkstatus` command has been added
```bash
./bat-go settlement gemini checkstatus --input=bulk-signed-transactions.json --all-txs-input=from-antifraud.json
```
