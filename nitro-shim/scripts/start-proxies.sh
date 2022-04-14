#!/bin/bash

CID=""

# first get the cid of the enclave
# wait for enclave to startup
for i in `seq 0 5`
do
        sleep 20
        CID=$(nitro-cli describe-enclaves | jq -r .EnclaveCID)
        if [ "${CID}" == "" ]; then
                continue
        fi
        break
done

# at this point the enclave is up
# setup inbound traffic proxy
IN_ADDRS=":8080"
OUT_ADDRS="${CID}:8080"

# next startup the proxy
/enclave/viproxy > /tmp/viproxy.log &
