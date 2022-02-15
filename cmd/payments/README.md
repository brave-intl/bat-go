# Payments Service

This service has two endpoints, Prepare and Submit.  Prepare takes passed in transactions
and stores them in QLDB in prepared state.  Submit takes the list of passed in transactions
verifies that the signature on the request matches one of the allowed authorizers, and is valid
and then checks that the values of the transaction match what is in QLDB, verifying that the
transaction data has not been tampered with, then performs a transaction submission to the
custodian.

## Endpoints

### Prepare

```http
POST /v1/payments/{custodian}/prepare
[
  { idempotencyKey: <uuid>, amount: <decimal>, to: <identifier>, from: <identifier> }
  ...
]

HTTP/1.1 200
[
  { idempotencyKey: <uuid>, amount: <decimal>, to: <identifier>, from: <identifier>, documentId: <identifier> }
  ...
]
```

The caller will perform a `POST` request to the `/v1/payments/{custodian}/prepare` endpoint with 
a JSON array of transactions.  Transactions will consist of an idempotencyKey which will be passed along to
the custodians to make sure this transaction only happens once, an amount in decimal form, to which is where the
funds should go (uphold card, gemini recipient id, bitflyer deposit id) and a from which is a brave owned card in uphold's case,
or a sub account in gemini's case.  The URL parameter custodian will indicate which custodian these transactions should
be performed against.

Returned is the exact same array with the exception that every single record will have a document id which is the qldb
identifier, used to verify the transaction data is correct and has not been tampered with.

### Submit

```http
POST /v1/payments/submit
DIGEST: ...
SIGNATURE: ...
[
  { idempotencyKey: <uuid>, amount: <decimal>, to: <identifier>, from: <identifier>, documentId: <identifier> }
  ...
]

HTTP/1.1 200
```

The caller will perform a `POST` request to the `/v1/payments/submit` endpoint with the response from the prepare API call.
This request will employ an http signature from a hard coded set of valid http signers called authorizers.  The signature will
employ the ed25519 signature scheme currently employed for other services.

