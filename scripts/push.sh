#!/bin/env bash

for service in $(find . -type f -name Dockerfile | awk -F'/' '{print $2}'); do
	if [[ ${service} -ne "Dockerfile" ]]; then
        repo=$(echo ${IMAGE_TAG} | awk -F':' '{print $1}')
        tag=$(echo ${IMAGE_TAG} | awk -F':' '{print $2}')
        srv_image_tag="${repo}/${service}:${tag}"
        docker push ${srv_image_tag}
	fi
done;
