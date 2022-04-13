#!/bin/bash

docker_image_base="${1}"

# service var is the service we wish to run in the enclave
service=""
if [ "${2}" != "" ]; then
    service="/${2}"
fi

and_run="${3}"

set -euxo pipefail

# get the latest docker image of the base image we are looking for
docker_image=$(docker images --format "{{.Repository}} {{.Tag}} {{.CreatedAt}}" | grep "${docker_image_base}" | sort -rk 3 | awk -v s="${service}" 'NR==1{printf "%s%s:%s", $1, s, $2}')

nitro-cli build-enclave --docker-uri ${docker_image} --output-file nitro-image.eif

if [ "${and_run}" == "run" ]; then 
  /enclave/run.sh
fi

