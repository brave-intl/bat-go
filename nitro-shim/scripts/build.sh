#!/bin/bash

docker_image_base="${1}"
and_run="${2}"

set -euxo pipefail

docker_image=$(docker images | grep "${docker_image_base}" | awk '{printf "%s:%s", $1, $2}')

nitro-cli build-enclave --docker-uri ${docker_image} --output-file nitro-image.eif

if [ "${and_run}" == "run" ]; then 
  /enclave/run.sh
fi

