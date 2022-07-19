#!/bin/bash

service="${1}"
CID=""
PARENT_CID="3" # the CID of the EC2 instance

# first get the cid of the enclave
# wait for enclave to startup
for i in `seq 0 5`
do
        sleep 20
        CID=$(nitro-cli describe-enclaves | jq -r .[].EnclaveCID)
        if [ "${CID}" == "" ]; then
                continue
        fi
        break
done

# at this point the enclave is up.  depending on what service we're running,
# it's now time to set up proxy tools
if [ "${service}" = "/payments" ]; then
    # setup inbound traffic proxy
    export IN_ADDRS=":8080"
    export OUT_ADDRS="${CID}:8080"
elif [ "${service}" = "/ia2" ]; then
    # setup proxy that allows the enclave to talk to Let's Encrypt and our Kafka
    # cluster
    export SOCKS_PROXY_ALLOWED_FQDNS="acme-v02.api.letsencrypt.org,${KAFKA_BROKERS}"
    export SOCKS_PROXY_ALLOWED_ADDRS=""
    export SOCKS_PROXY_LISTEN_ADDR="127.0.0.1:1080"
    /enclave/socksproxy > /tmp/socksproxy.log &

    # setup proxy that serves as an HTTP-to-Kafka bridge
    export KAFKA_KEY_PATH="/etc/kafka/secrets/key"
    export KAFKA_CERT_PATH="/etc/kafka/secrets/certificate"
    export KAFKA_PROXY_LISTEN_ADDR="127.0.0.1:8081"
    export KAFKA_TOPIC="ip_addr_anon.dev.repsys.upstream"
    /enclave/kafkaproxy > /tmp/kafkaproxy.log &

    # setup proxy for inbound traffic, ACME, and the enclave's SOCKS proxy
    export IN_ADDRS=":8080,:80,${PARENT_CID}:80"
    export OUT_ADDRS="${CID}:8080,${CID}:80,127.0.0.1:1080"
fi

# next startup the proxy
/enclave/viproxy > /tmp/viproxy.log &
