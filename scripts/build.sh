#!/bin/env bash

for service in $(find . -type f -name Dockerfile | awk -F'/' '{print $2}'); do
	if [ ${service} != "Dockerfile" ]; then
        echo "building ${service}";
		# named service, build with that dockerfile
		# perform surgery on the IMAGE_TAG to name the container right
		repo=$(echo ${IMAGE_TAG} | awk -F':' '{print $1}')
		tag=$(echo ${IMAGE_TAG} | awk -F':' '{print $2}')
		srv_image_tag="${repo}/${service}:${tag}"

        # sub out the correct repository based on the repo build that just happened (image_tag)
        repl="s/FROM.*$/FROM ${IMAGE_TAG//\//\\\/}/g"
        sed -i "$repl" ${service}/Dockerfile

		docker run -v $(pwd):/workspace --network=host gcr.io/kaniko-project/executor:v1.6.0 --reproducible --dockerfile /workspace/${service}/Dockerfile --no-push --tarPath image-${service}-file.tar --destination ${srv_image_tag} --context="dir:///workspace/"
		cat image-${service}-file.tar | docker load
	fi
done;
