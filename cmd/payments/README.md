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

#### Error Conditions

- 400 - Non-Retriable Errors
  - Custodian URL Parameter Invalid
  - Transaction List is inproperly formatted
- 500 - Retriable Error
  - Misconfigured Service
  - Unrecoverable Server Error
- 503 - Retriable Error
  - Service not available

#### Common Error Response Structure

```json
{
    "message": <string>, // will include a human readable message about the cause of the error
    "code": <int>, // the application specific error coding
    "data": <object> // context data about the error
}
```



### Submit

```http
POST /v1/payments/submit
DIGEST: ...
SIGNATURE: ...
[
  { idempotencyKey: <uuid>, amount: <decimal>, to: <identifier>, from: <identifier>, documentId: <identifier> }
  ...
]

HTTP/1.1 202
```

The caller will perform a `POST` request to the `/v1/payments/submit` endpoint with the response from the prepare API call.
This request will employ an http signature from a hard coded set of valid http signers called authorizers.  The signature will
employ the ed25519 signature scheme currently employed for other services.

The submit endpoint will asynchronously process transactions.  To get the individual status of the transactions
one must use the Get Status endpoint discussed below to get the full response from the custodian.

#### Error Conditions

- 400 - Non-retriable Error
  - Transaction List is inproperly formatted
  - Transaction List has been tampered with
  - Upstream Custodian Validation Errors
- 403 - Non-retriable error
  - Unauthorized Submit, Verifier is not acceptable, Signature Invalid
- 500 - Retriable Error
  - Misconfigured Service
  - Unrecoverable Server Error
  - Upstream Custodian Transient Server Errors
- 503 - Retriable Error
  - Service not available

#### Common Error Response Structure

```json
{
    "message": <string>, // will include a human readable message about the cause of the error
    "code": <int>, // the application specific error coding
    "data": <object> // context data about the error
}
```

Example below:

```json
{
    "message": "failed to accept transaction",
    "code": 400,
    "data": {
        "transactions": [
            {
                "transaction": {
                        idempotencyKey: <uuid>,
                        amount: <decimal>,
                        to: <identifier>,
                        from: <identifier>,
                        documentId: <identifier>
                },
                "reason": "invalid documentId"
            }
        ]
    }
}
```

### Get Status

```http
POST /v1/payments/get-status
[
  <document id>,
  ...
]

HTTP/1.1 200
[
  {
    "transaction": { idempotencyKey: <uuid>, amount: <decimal>, to: <identifier>, from: <identifier>, documentId: <identifier> },
    "submissionResponse": <custodian response>
    "statusResponse": <custodian response>
    "status": (completed | pending | processing | failed)
  },
  ...
]
```

#### Error Conditions

- 400 - Non-Retriable Errors
  - invalid document ids
  - Transaction List is inproperly formatted
- 500 - Retriable Error
  - Misconfigured Service
  - Unrecoverable Server Error
- 503 - Retriable Error
  - Service not available

#### Common Error Response Structure

```json
{
    "message": <string>, // will include a human readable message about the cause of the error
    "code": <int>, // the application specific error coding
    "data": <object> // context data about the error
}
```
