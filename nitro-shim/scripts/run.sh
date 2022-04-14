#!/bin/bash

set -euxo pipefail

nitro-cli run-enclave --cpu-count 2 --memory 512 ---eif-path nitro-image.eif > /tmp/output.json
cat /tmp/output.json

/enclave/start-proxies.sh &

# sleep forever while enclave runs
/enclave/sleep.sh

