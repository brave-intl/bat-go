Below is the command structure for bat-go microservices using cobra

```
./bat-go
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
