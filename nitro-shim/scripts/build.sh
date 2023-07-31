#!/bin/bash

docker_image_base="${1}"

# service var is the service we wish to run in the enclave
service=""
if [ "${2}" != "" ]; then
    service="/${2}"
fi

and_run="${3}"
run_cpu_count="${4}"
run_memory="${5}"

set -eux

# wait for a few seconds for eks to pull down the right version
sleep 20

# get the latest docker image of the base image we are looking for
docker_image=$(docker images --format "{{.Repository}} {{.CreatedAt}}" | grep "${docker_image_base}" | sort -rk 2 | awk 'NR==1{printf "%s", $1}')

if [ -z "${docker_image}" ]; then
    docker_image=${docker_image_base}
fi

aws ecr get-login-password --region us-west-2 | docker login --username AWS --password-stdin ${docker_image}

# get the latest docker image of the base image we are looking for with tag
docker_image_tag=$(docker images --format "{{.Repository}} {{.Tag}} {{.CreatedAt}}" | grep "${docker_image_base}" | sort -rk 3 | awk 'NR==1{printf "%s:%s", $1, $2}')
if [ -z "${docker_image_tag}" ]; then
    docker_image_tag=${docker_image_base}
fi

if [[ ! -z "$EIF_PASS_ENV" && ! -z "$EIF_COMMAND" ]]; then
  /enclave/eifbuild -pass-env $EIF_PASS_ENV -docker-uri ${docker_image_tag} -output-file nitro-image.eif -- sh -c \"$EIF_COMMAND\"
else
  nitro-cli build-enclave --docker-uri ${docker_image_tag} --output-file nitro-image.eif
fi

if [ "${and_run}" == "run" ]; then
  /enclave/run.sh "${service}" ${run_cpu_count} ${run_memory}
fi

