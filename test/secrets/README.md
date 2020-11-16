Generated using https://github.com/confluentinc/cp-docker-images/blob/5.3.1-post/examples/kafka-mqtt-single-node-ssl-producer/secrets/create-certs.sh and generate-client.sh

In the create-certs.sh script, when creating the keystores, change the
distinguished name (-dname argument) to have CN=kafka, and also CN=localhost if
you want to access kafka over TLS outside of the docker subnet.

run
```bash
rm broker1* consumer* kafka* snakeoil-ca*
./create-certs.sh
./generate-client.sh
```
when the cert expires next
