#!/bin/bash

set -euxo pipefail

nitro-cli run-enclave --eif-path nitro-image.eif --debug-mode > /tmp/output.json
cat /tmp/output.json
EID=$(cat /tmp/output.json | jq -r .EnclaveID)

# sleep forever while enclave runs
/enclave/sleep.sh

