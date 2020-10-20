Below is the command structure for bat-go microservices using cobra

```bash
./bat-go macaroon create --config=config.yaml
```

```
./bat-go
  - macaroon
    - create
      example:
          macaroon create \
            --config=config.yaml \
            --secret=mysecret # MACAROON_SECRET env
  - serve
    - rewards
      - rest
        example:
            serve rewards rest \
              --ratios-token "abc" --ratios-service "123" --environment "local" \
              --base-currency "USD" --address ":4321"
      - grpc
        example:
            serve rewards grpc \
              --ratios-token "abc" --ratios-service "123" --environment "local" \
              --base-currency "USD" --address ":4321"
    - getcertfingerprint
      example:
            serve getcertfingerprint api.uphold.com:443
  - settlement
    - paypal
      - transform
      - complete
      - email
    - gemini
      - sign
      - submit
  - wallet
    - create
      example:
          wallet create \
            --provider=uphold --name=test
    - transfer-funds
      example:
          wallet transfer-funds \
            --provider=uphold --from=1234567890 --to=1234567890 --usevault=true \
            --value=10.5
  - vault
    - init
      example:
          vault init -key-shares=1 \
            -key-threshold=1 \
            ./key.asc
    - import-key
      exports:
        * ED25519_PRIVATE_KEY
        * ED25519_PUBLIC_KEY
        * UPHOLD_PROVIDER_ID
        * GEMINI_CLIENT_ID
        * GEMINI_CLIENT_KEY
        * GEMINI_CLIENT_SECRET
      example:
          vault import-key -c config.yaml \
            --wallet-refs gemini-referral
    - sign-settlement
      example:
          vault sign-settlement -c config.yaml \
            -i contributions.json \
            --providers=uphold,gemini
    - unseal
      example:
          gpg -d ./share-0.gpg | bat-go vault unseal
    - create-wallet
      example:
          bat-go vault create-wallet
```
