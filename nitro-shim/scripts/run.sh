#!/bin/bash

service="${1}"
cpu_count=${2:-2}
memory=${3:-512}
cid="4"

set -eux

nitro-cli run-enclave \
    --enclave-cid "${cid}" \
    --cpu-count ${cpu_count} \
    --memory ${memory} \
    --eif-path nitro-image.eif > /tmp/output.json
cat /tmp/output.json

# background the proxy startup
/enclave/start-proxies.sh "${service}" "${cid}" &

# sleep forever while enclave runs
# will cause the container to die if enclave dies
/enclave/sleep.sh

