## Creating new local vault instance

```
# add vault address into profile or similar for your shell
echo "export VAULT_ADDR=http://127.0.0.1:8200" >> ~/.profile

# set in local shell
export VAULT_ADDR=http://127.0.0.1:8200

# start vault server
vault server -config=config.hcl

# initialize
vault-init -key-shares=1 -key-threshold=1 GPG_PUB_KEY_FILE...

# unseal vault
gpg -d SHARE.GPG | vault-unseal
```

## Running settlement

On the offline computer, in one window run:
```
vault server -config=config.hcl
```

In another run:
```
gpg -d SHARE.GPG | vault-unseal
```

You are now ready to transact
```
vault-sign-settlement -in <SETTLEMENT_REPORT.JSON> <SETTLEMENT_WALLET_CARD_ID>
```

Finally seal the vault:
```
vault operator seal
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
