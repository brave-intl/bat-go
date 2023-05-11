#!/bin/bash

openssl genrsa -out consumer.client.key 1024
openssl req -key consumer.client.key -new -out consumer.client.req -subj '/OU=TEST/O=CONFLUENT/L=PaloAlto/S=Ca/C=US' -addext "subjectAltName = DNS:consumer.test.confluent.io"
openssl x509 -req -CA snakeoil-ca-1.crt -CAkey snakeoil-ca-1.key -in consumer.client.req -out consumer-ca1-signed.pem -days 9999 -CAcreateserial -extfile v3.ext
