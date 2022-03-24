#!/bin/bash

docker_image="${1}"
and_run="${2}"

set -euxo pipefail

nitro-cli build-enclave --docker-uri ${docker_image} --output-file nitro-image.eif

if [ "${and_run}" == "run" ]; then 
  /enclave/run.sh
fi

