#!/bin/bash

set -o nounset \
    -o errexit \
    -o verbose \
    -o xtrace

# key generation for payments authorization endpoint
# each individual authorizer will have their own ed25519 keypair which they
# will use to sign the authorize request, thereby signing off on the batch
# openssl genpkey -algorithm ed25519 -outform PEM -out test/secrets/test-auth-key.pem
# get public key for payments service
# openssl pkey -outform DER -pubout -in test/secrets/test-auth-key.pem | tail -c +13 | xxd -p -c32


# Generate CA key
openssl req -new -x509 -keyout snakeoil-ca-1.key -out snakeoil-ca-1.crt -days 365 -subj '/CN=ca1.test.confluent.io/OU=TEST/O=CONFLUENT/L=PaloAlto/S=Ca/C=US' -passin pass:confluent -passout pass:confluent
# openssl req -new -x509 -keyout snakeoil-ca-2.key -out snakeoil-ca-2.crt -days 365 -subj '/CN=ca2.test.confluent.io/OU=TEST/O=CONFLUENT/L=PaloAlto/S=Ca/C=US' -passin pass:confluent -passout pass:confluent


# client/server certificates for payments grpc service
openssl genrsa -out payments.client.key 1024
openssl req -key payments.client.key -new -out payments.client.req -subj '/CN=payments.test/OU=TEST/O=Payments/L=PaloAlto/S=Ca/C=US'
openssl x509 -req -CA snakeoil-ca-1.crt -CAkey snakeoil-ca-1.key -in payments.client.req -out payments-client-ca1-signed.pem -days 9999 -CAcreateserial -passin "pass:confluent"

openssl genrsa -out payments.server.key 1024
openssl req -key payments.server.key -new -out payments.server.req -subj '/CN=payments-server/OU=TEST/O=Payments/L=PaloAlto/S=Ca/C=US'
openssl x509 -req -CA snakeoil-ca-1.crt -CAkey snakeoil-ca-1.key -in payments.server.req -out payments-server-ca1-signed.pem -days 9999 -CAcreateserial -passin "pass:confluent"



# # Kafkacat
# openssl genrsa -des3 -passout "pass:confluent" -out kafkacat.client.key 1024
# openssl req -passin "pass:confluent" -passout "pass:confluent" -key kafkacat.client.key -new -out kafkacat.client.req -subj '/CN=kafkacat.test.confluent.io/OU=TEST/O=CONFLUENT/L=PaloAlto/S=Ca/C=US'
# openssl x509 -req -CA snakeoil-ca-1.crt -CAkey snakeoil-ca-1.key -in kafkacat.client.req -out kafkacat-ca1-signed.pem -days 9999 -CAcreateserial -passin "pass:confluent"

# used to be
# broker1 producer consumer
for i in broker1 producer consumer
do
	echo $i
	# Create keystores
	keytool -genkey -noprompt \
				 -alias $i \
				 -dname "CN=kafka, OU=TEST, O=CONFLUENT, L=PaloAlto, S=Ca, C=US" \
				 -keystore kafka.$i.keystore.jks \
				 -keyalg RSA \
				 -storepass confluent \
				 -keypass confluent

	# Create CSR, sign the key and import back into keystore
	keytool -keystore kafka.$i.keystore.jks -alias $i -certreq -file $i.csr -storepass confluent -keypass confluent

	openssl x509 -req -CA snakeoil-ca-1.crt -CAkey snakeoil-ca-1.key -in $i.csr -out $i-ca1-signed.crt -days 9999 -CAcreateserial -passin pass:confluent

	keytool -keystore kafka.$i.keystore.jks -alias CARoot -import -file snakeoil-ca-1.crt -storepass confluent -keypass confluent

	keytool -keystore kafka.$i.keystore.jks -alias $i -import -file $i-ca1-signed.crt -storepass confluent -keypass confluent

	# Create truststore and import the CA cert.
	keytool -keystore kafka.$i.truststore.jks -alias CARoot -import -file snakeoil-ca-1.crt -storepass confluent -keypass confluent

  echo "confluent" > ${i}_sslkey_creds
  echo "confluent" > ${i}_keystore_creds
  echo "confluent" > ${i}_truststore_creds
done
