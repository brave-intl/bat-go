# Settlement CLI

The settlement CLI tooling allows settlement operators to enqueue
validate, and authorize custodian transactions.

### Setup Redis Locally
```bash
// add `127.0.0.1 redis` to hosts file
docker-compose -f docker-compose.redis.yml up -d # to start up the local redis cluster
```

## Commands

Available commands are:

1. `prepare`
2. `bootstrap`
3. `authorize`
4. `validate`
5. `enable`

### Enable

For the payments service to download relevant configurations, the operator will need to `enable`
the system by encrypting their operator share (from bootstrap command) with a KMS key that only the enclave can decrypt from.
Below is an example of how to run:

```bash
    aws-vault exec <operator aws role> -- \
        go run main.go enable \
            --kms-key="arn:aws:kms:*******:key/**********" \
            --s3-bucket="*****************" \
            --share="ba24835f48f31914a853588b6a0297c78b167ee98e95af2c02c1e1b44452b00f28"
```

This command will check the enclave measurements match the key policy for the kms encryption key, and
then encrypt this share with the kms key and upload it to the enclave s3 bucket.  The payments service
will wait for two objects to show up in the bucket that it can decrypt with KMS and then use the values
to combine the Shamir shares and decrypt the bootstrap ciphertext.


### Bootstrap

For the payments service to download relevant configurations, the operator will need to bootstrap
the system by encrypting the configurations with a KMS key that only the enclave can decrypt from,
so the bootstrap command takes the configurations and performs the encryption and uploads the
configuration to s3.  Below is an example of how to run:

```bash
    aws-vault exec <operator aws role> -- \
        go run main.go bootstrap \
            --kms-key="arn:aws:kms:*******:key/**********" \
            --s3-bucket="*****************" \
            --bootstrap-file=test/bootstrap.json
```

bootstrap.json should be structured in a way that the payments service initialization is able to read
parse and use it as it's configuration.  Output from this command will also be 5 shamir key shares with
a threshold of 2 to reconstitute a randomly generated key which is used to encrypt the bootstrap file.

The output will look something like this:

```bash
7:44AM INF bootstrap created encryption key... module=bootstrap
7:44AM INF bootstrap created key shares for operators... module=bootstrap
        !!! OPERATOR SHARE 0: ba24835f48f31914a853588b6a0297c78b167ee98e95af2c02c1e1b44452b00f28
        !!! OPERATOR SHARE 1: 1de3cbd74c18020447dd421fe63ade57ff45b5440f9df7bd7d375346bee85171e9
        !!! OPERATOR SHARE 2: 75127e4bec6597b29ab1ffcedaa043267065bf27e0c6f4e4d12f05fe5de527f5e8
        !!! OPERATOR SHARE 3: 672b81e207635f33770b4bc098f5c0c33152a000f80b8a7dd9f201c8a62c1481c2
        !!! OPERATOR SHARE 4: 36d990aee3c77a8e37d1570630fae8e1cb6665f05ed82666bbe5303b86ce11daa6
```

These operator shares should be discretely handed out to the various payment operators, two of which
at any given time will need to perform the "enable" command with their secret share to allow a new
enclave to process the secrets in the bootstrap file.

### Prepare

The prepare command parses the payout report, and enqueues the transactions in 
a new per payout stream identified by `--payout-id` parameter.  Tool uses redis streams
to connect to `--redis-addrs` and `--redis-user` as well as `REDIS_PASS` env variable
to connect to redis to submit transactions for preparation.

The payout report looks like the below:
```json
[
  {
    "address": "a6a5ff0c-f45e-40ac-8ed3-b2bc32454066",
    "probi": "3270000000000030000", # this is the value of the transaction in BAT probi
    "publisher": "wallet:ab7198e8-5ee3-4626-8315-c7f2ace8f1c2",
    "transactionId":"8d2c3616-d582-4d00-9d7d-a300a8f041d6", # this is the batch identifier
    "walletProvider": "uphold"
  },
  ...
]
```

After this payout file is parsed, and transformed, prepared transactions are submitted
to the settlement workers via redis streams, and the prepare workers are then configured
to find that payout stream to start preparing the transactions.  Below is an example of
how this is run:

```bash
REDIS_PASS=whatever_the_pass_is \
    go run main.go prepare \
        --report test/report.json \
        --payout-id 20230202_1 \
        --redis-addrs redis:6380,redis:6383,redis:6381,redis:6382 \
        --redis-user redis \
        --test-mode # test mode is just for testing, not production
```

### Validate

The validate command parses the original payout report, and the attested report which is downloaded
after settlement workers complete preparation.  The attested report has the following structure:

```json
[
    {
        "to":"a6a5ff0c-f45e-40ac-8ed3-b2bc32454066",
        "amount":"3.27000000000003",
        "idempotencyKey":"1d691291-f376-5591-8ab8-43db145d0e5e",
        "custodian":"uphold",
        "documentId":"1234",
        "attestationDocument": "a4f1354071880faee6391a022b120471e1254afbc87f198d4ce8833350b3a9596fb09ea17eb20fcfc0ed8e63596281cca4f260096943bc6eadf78ffef6da5604"
    },
    ...
]
```

The documentId is the QLDB document identifier for the transaction record stored by payments
service from the enclave.  The attestation document is from nitro, and the userdata should be validated against
the transaction.  The following signing string is used for the userdata in the attestation document:

```go
// BuildSigningString - the string format that payments will sign over per tx
func (at AuthorizeTx) BuildSigningBytes() []byte {
    return []byte(fmt.Sprintf("%s|%s|%s|%s|%s",
        at.To, at.Amount.String(), at.ID, at.Custodian, at.DocumentID))
}
```

The validate command is run by the operator prior to the authorize command.  This command
performs the following feats:

1. validates the number of transactions in the original report matches the attested report
2. validates the amount of bat in the original report matches the attested report
3. validates each transaction was attested by nitro through the payments service running in the enclave
4. outputs based on custodian the amount of total BAT being paid out.

If the validate command completes successfully, the operator can spot check the values manually
and then perform the authorize command.  Below is an example of how to run the validate command:

```bash
go run main.go validate \
    --report test/report.json \
    --attested-report test/attested-report.json \
    --nitro-cert ./certs/root.pem \
    --test-mode # test mode just for testing, not production
```

### Authorize

The authorize command is used by a payments operator to inform the system that
the transactions in the attested-report are ready to be paid out.  The authorize command
parses the attested-report, and for each transaction creates an httpsignature using the
transaction json as the payload body.  The header and body values are passed then to the settlement
submit workers for submission to the payments service in nitro.

The settlement workers will need to use the values passed in for headers, and pass along the signature,
which has the public key of the operator in the keyId field of the Signature header.

The headers used for the httpsignature computation are:

* (request-target)
* Host
* Digest // the sha256 over the body payload
* Content-Length
* Content-Type

Note: we are not using Date as at signing time we don't know what it will be.

```bash
REDIS_PASS=whatever_the_pass_is 
    go run main.go authorize \
    --attested-report test/attested-report.json \ # the attested report you validated
    --payout-id 20230202_1 \ # identifier of the payout
    --redis-addrs redis:6380,redis:6383,redis:6381,redis:6382 \
    --redis-user redis \ 
    --key-file test/private.pem \ # this is your operator key, payments validates your key
    --payments-host https://payments.bsg.brave.software \ # this is the host of the payments service in nitro
    --test-mode # this is just for testing not prod
```

Business logic in payments service may require multiple independent operators to sign the transactions
submitted prior to payout.
