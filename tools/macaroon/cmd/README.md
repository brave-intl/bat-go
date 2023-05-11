# Macaroon CLI Generation

This tool provides a mechanism for creation of macaroon tokens from the command line
provided there is a yaml file defining all of the tokens one wishes to generate, as
well as the HMAC secret key.

```bash
cd bat-go/ # go to root of bat-go repository
go build ./... # build the bat-go exec
./bat-go macaroon create --config cmd/macaroon/brave-talk/brave_talk_free_dev.yaml --secret secret
```

The above command will read "example.yaml" parse it into a token config and then create
macaroons based on the tokens specified in the yaml file.  Below is an example yaml file:

```yaml
---
tokens:
  - 
      id: "brave user-wallet-vote sku token v2"
      version: 2
      location: "brave.com"
      first_party_caveats:
          - id: "ef0db4e2-c247-4b9b-99ab-5fc72213ac3a"
          - sku: "user-wallet-vote"
  - 
      id: "brave anon-card-vote sku token v2"
      version: 2
      location: "brave.com"
      first_party_caveats:
          - id: "5cbe9620-ca98-4d09-8c18-1b8582d78e60"
          - sku: "anon-card-vote"
```
