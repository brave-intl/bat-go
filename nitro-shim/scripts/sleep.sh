#!/bin/bash

set -euxo pipefail

while true
do
        # check every minute that the enclave is running
        sleep 60

        EID= $(nitro-cli describe-enclaves | jq -r .EnclaveID)
        if [ "${EID}" == "" ]; then
                break;
        fi
done
