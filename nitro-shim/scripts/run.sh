#!/bin/bash

service="${1}"

set -euxo pipefail

nitro-cli run-enclave --cpu-count 2 --memory 512 ---eif-path nitro-image.eif > /tmp/output.json
cat /tmp/output.json

# background the proxy startup
/enclave/start-proxies.sh "${service}" &

# sleep forever while enclave runs
# will cause the container to die if enclave dies
/enclave/sleep.sh

