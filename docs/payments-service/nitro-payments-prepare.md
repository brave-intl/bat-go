# Payments Service Prepare

The prepare method is used to submit a payout for authorization.  The payments service
employs a GRPC endpoint, the method of which is `Prepare` and is [defined here](../../protos/payments-api.proto).

## PrepareRequest Structure

The `Prepare` method takes as a parameter a `PrepareRequest` structure shown below:

```go
import paymentspb "github.com/brave-intl/bat-go/payments/pb"

req := paymentspb.PrepareRequest{
    Custodian: "uphold",
    BatchMeta: paymentspb.BatchMeta {
        // if batch id is not defined, service will define one for request
        // if using multiple pages, define the batch id as the same on each request
        BatchID: uuid.NewV4().String(),
    },
    BatchTxs: []*paymentspb.Transaction{
        &paymentspb.Transaction{
            IdempotencyKey: "this is the idempotency key for this transaction",
            Destination: "this is the destination card id for the custodian user",
            Origin: "this is the origin card id, such as settlement address",
            Amount: "42.424242424242424",
            Currency: "this is the currency",
            Metadata: []*paymentspb.ContextItem { // metadata is just extra stuff, like notes, creator info, etc
                &paymentspb.ContextItem{
                    Key: "creator",
                    Value: "very-popular-creator",
                },
                // ...
            },
        },
        // ...
    },
}
```

## PrepareResponse Structure

The `Prepare` method results in a `PrepareResponse` structure shown below:

```go
import paymentspb "github.com/brave-intl/bat-go/payments/pb"

res := paymentspb.PrepareResponse{
    Meta: &MetaResponse{
        Status: paymentspb.SUCCESS,
        Msg: "message explaining status of request",
        Context: []*paymentspb.ContextItem{
            &paymentspb.ContextItem{
                Key: "some metadata context key",
                Value: "some metadata context value",
            },
            // ...
        },
    },
    BatchId: "this is the batch ID for the batch",
    AuthUrl: "this is the url of the full transaction list for authorizers to validate prior to authing",
    DocumentIds: []string { // this will get updated every time you submit more transactions to prepare prior to authorization
        "QLDB document ids for all transactions in the batch",
        // ...
    }
}

```

## Notes

Please note the following:

1. Transactions within Prepare Requests:
    1. Are appended to batches based on batch id.  You can call prepare request as many times as you want to append more transactions
    2. Are idempotent based on the idempotency key, meaning if you resubmit it will not add that transaction again to the batch
2. Each time you call Prepare request:
    1. You will get a response that contains all of the document ids (QLDB key) of the transactions that have been submitted to batch
    2. The response will include a link to an "auth url" which is a signed (by payments service) object enumerating the full batch
    3. The response will include metadata explaining the request status, explaination, and any associated context
    4. Use the GRPC status codes to determine if you need to re-run the prepare for a transaction batch
3. Once an Authorization happens on a batch, the batch is immutable.
    1. Be sure to have submitted all transactions prior to notification of authorizers to authorize the batch.
