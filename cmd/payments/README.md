# Payments CLI Tooling

### Load Command

```bash
../bat-go payments load s3://${payout-transactions-file}.json
```

Payments load command takes as a parameter the s3 URI where the
valid payout transaction are located.  The internals of this command
then parse the transactions, and call the `prepare` GRPC endpoint
of the payments service enumerating all of the transactions.

This command will most likely be run from payout automation scripting.

### Authorize Command

```bash
../bat-go payments authorize ${batch_document_uuid} \\
    --private-key-path=${path_of_my_private_key} --public-key-id=${public key uuid}
```

The authorize subcommand will perform a digest/signature on the batch document uuid
stated and use the output digest/signature as well as the batch id and public key id
in an authorization request.  This request will be sent to the authorize GRPC handler
in the payments service which will denote all authorizations for the given batch
document uuid.

In order to perform the submit command, n-number of authorizations must be performed
dictated by the business logic in the submit handler for acceptance and processing
of a batch of transactions.  This will allow us to have a multiple key turn scheme.

The intended function of this command is NOT to be run by automation, but rather be manually
invoked by authorized key holders.

### Submit Command

```bash
../bat-go payments submit ${batch_document_uuid}
```

The submit handler will take as a parameter a batch document uuid and submit a request to the
payments' GRPC service informing it that we intend to submit this batch of transactions to
the custodian.

This is an asynchronous call, and the intended response is an indication that the job was accepted
or not accepted for processing.  Some reasons for not accepting could be: not enough authorizations,
invalid batch document uuid, or any number of server error conditions leading to the submission not
being accepted.
