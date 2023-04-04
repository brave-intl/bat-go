# Payments CLI

The payments CLI tooling allows settlement operators to enqueue
validate, bootstrap, configure and authorize custodian transactions.

## To Build

In order to build the cli tools run the following in this current directory.

```bash
make
```

If you make changes an easy way to bring each of the individual tool's dependencies up to date you
can run the following:
```bash
make tidy
```

If you want to start clean, to remove the go cache and resulting `dist` binary files, run the following:

```bash
make clean
```

### Setup Redis Locally
```bash
// add `127.0.0.1 redis` to hosts file
docker-compose -f redistest/docker-compose.redis.yml up -d # to start up the local redis cluster
```

## Commands

Available commands are:

1. `create-vault`
2. `configure`
3. `bootstrap`
4. `prepare`
5. `validate`
6. `authorize`

### Create Vault

Create generates a random asymmetric key pair, breaks the private key into operator shares, and outputs
the number of shares at a given threshold to standard out, along with the public key.  After this is
performed the private key is discarded.

Create takes as parameters the threshold and number of operator shares.

```
Usage:

create-vault [flags]

The flags are:

	-t
		The Shamir share threshold to reconstitute the private key
	-n
		The number of operator shares to output
```

### Configure
Configure encrypts a configuration file for consumption by the payments service, the output of which
is then uploaded to s3 and consumed by the payments service.

Create takes as parameters the public key output from the create command, and a configuration file.

```
Usage:

configure [flags] file [files]

The flags are:

	-k
		The public key of the payments service (output from create command)

The arguments are configuration files which are to be encrypted.
```

The resulting encrypted files are only able to be decrypted by the enclave when two operators
perform the bootstrap command.


### Bootstrap

Bootstrap takes the provided operator shamir key share and encrypts the key with
the provided KMS encryption key (that only the enclave can decrypt with) and then
uploads the ciphertext to s3 for the enclave to download

Bootstrap takes as parameters the operator share, kms key arn and s3 uri.

```
Usage:

bootstrap [flags]

The flags are:

	-s
		The operator's Shamir key share from the create command
	-k
		The KMS Key ARN to encrypt the key share with
	-b
		The S3 URI to upload the ciphertext to
```

This command will check the enclave measurements match the key policy for the kms encryption key, and
then encrypt this share with the kms key and upload it to the enclave s3 bucket.  The payments service
will wait for two objects to show up in the bucket that it can decrypt with KMS and then use the values
to combine the Shamir shares and decrypt the bootstrap ciphertext.


### Prepare

The prepare command parses the payout report, and enqueues the transactions in 
a new per payout stream.  Tool uses redis streams
to connect to `-ra` and `-ru` as well as `-rp` variable
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
Usage:

prepare [flags] filename...

The flags are:

	-v
		verbose logging enabled
	-e
		The environment to which the operator is sending transactions to be put in prepared state.
		The environment is specified as the base URI of the payments service running in the
		nitro enclave.  This should include the protocol, and host at the minimum.  Example:
			https://payments.bsg.brave.software
	-ra
		The redis cluster addresses comma seperated
	-rp
		The redis cluster password
	-ru
		The redis cluster user
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
the transaction. 
```
Usage:

validate [flags]

The flags are:

	-v
		verbose logging enabled
	-ar
		Location on file system of the attested transaction report for signing
	-pr
		Location on file system of the original prepared report
```

The validate command is run by the operator prior to the authorize command.  This command
performs the following feats:

1. validates the number of transactions in the original report matches the attested report
2. validates the amount of bat in the original report matches the attested report
3. validates each transaction was attested by nitro through the payments service running in the enclave
4. outputs the amount of total BAT being paid out.

If the validate command completes successfully, the operator can spot check the values manually
and then perform the authorize command.  Below is an example of how to run the validate command:

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
Usage:

authorize [flags] filename...

The flags are:

	-v
		verbose logging enabled
	-k
		Location on file system of the operators private ED25519 signing key in PEM format.
	-e
		The environment to which the operator is sending approval for transactions.
		The environment is specified as the base URI of the payments service running in the
		nitro enclave.  This should include the protocol, and host at the minimum.  Example:
			https://payments.bsg.brave.software
	-ra
		The redis cluster addresses comma seperated
	-rp
		The redis cluster password
	-ru
		The redis cluster user
```
