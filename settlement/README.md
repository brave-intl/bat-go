## Creating new local vault instance

```
# add vault address into profile or similar for your shell
echo "export VAULT_ADDR=http://127.0.0.1:8200" >> ~/.profile

# set in local shell
export VAULT_ADDR=http://127.0.0.1:8200

# start vault server
./vault server -config=config.hcl

# initialize
./vault-init -key-shares=1 -key-threshold=1 GPG_PUB_KEY_FILE...

# unseal vault
gpg -d SHARE.GPG | ./vault-unseal
```

## Bringing up vault

On the offline computer, in one window run:
```
./vault server -config=config.hcl
```

In another run:
```
gpg -d SHARE.GPG | ./vault-unseal
```

## Running settlement

First bring up vault as described above.

```
./vault-sign-settlement -in <SETTLEMENT_REPORT.JSON> <SETTLEMENT_WALLET_CARD_ID>
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
vault-create-wallet -offline name-of-new-wallet
```

Copy the created `name-of-new-wallet-registration.json` file to the online
machine.

Re-run vault-create-wallet, this will submit the pre-signed registration:
```
export UPHOLD_ENVIRONMENT=
export UPHOLD_HTTP_PROXY=
export UPHOLD_ACCESS_TOKEN=
vault-create-wallet -offline name-of-new-wallet
```

Finally copy `name-of-new-wallet-registration.json` back to the offline
machine and run vault-create-wallet to record the provider ID in vault:
```
vault-create-wallet -offline name-of-new-wallet
```

## Creating a config
the `config.example.yaml` should be copied wherever it is easiest to point to. just pass in the path while running a command that interacts with vault (import-key, sign-settlement) etc. be sure to change the values of the wallets to suit your setup if required.

## Importing keys

the following line imports the keys from environment variables
```bash
./vault-import-key
```

## Signing Files

signing the settlement file will split the input files into many output files depending on the contents of the file
```bash
./vault-sign-settlement -in=contributions.json
```

## Uploading files
running `settlement-submit` with a provider tells the script where to submit the file and the kind of handler to use. the sig=0 flag is for gemini bulk uploads that will need multiple submissions to check future status and create a completed list of transactions.
```bash
./settlement-submit -in=gemini-contributions-signed.json -provider=gemini -sig=0
```
