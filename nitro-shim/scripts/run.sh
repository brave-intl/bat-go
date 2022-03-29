#!/bin/bash

set -euxo pipefail

nitro-cli run-enclave --cpu-count 2 --memory 512 ---eif-path nitro-image.eif --debug-mode > /tmp/output.json
cat /tmp/output.json

# sleep forever while enclave runs
/enclave/sleep.sh

