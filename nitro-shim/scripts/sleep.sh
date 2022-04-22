#!/bin/bash

set -eux

while true
do
        # check every so often that the enclave is running
        sleep 480

        EID=$(nitro-cli describe-enclaves | jq -r .[].EnclaveID)
        if [ "${EID}" == "" ]; then
                break;
        fi
done
