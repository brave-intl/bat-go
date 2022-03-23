#!/bin/bash

and_run="${1}"
docker_image="${2}"

set -euxo pipefail

nitro-cli build-enclave --docker-uri ${docker_image} --output-file nitro-image.eif

if [ "${and_run}" == "run" ]; then
  /enclave/run.sh
fi

